package remotefiles

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cenkalti/backoff/v5"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/teapotovh/teapot/lib/files"
	"github.com/teapotovh/teapot/lib/run"
	"github.com/teapotovh/teapot/lib/s3cache"
	filespb "github.com/teapotovh/teapot/proto/files"
)

const (
	// DefaultPageSize is the default maximum number of inodes in a list page.
	DefaultPageSize = 1000

	// RevisionTooOld is the error code the files service sends when the
	// requested revision is no longer available.
	RevisionTooOld = "REVISION_TOO_OLD"
)

var (
	// ErrInvalidConfig reports a missing or invalid configuration.
	ErrInvalidConfig = errors.New("invalid config")
	// ErrInvalidInode reports an inode without an identifier.
	ErrInvalidInode = errors.New("invalid inode")
	// ErrInvalidEvent reports an event without a valid update or delete.
	ErrInvalidEvent = errors.New("invalid event")
	// ErrNoUID reports a context without a user id.
	ErrNoUID = errors.New("user id is not in the context")

	errSkip   = errors.New("event already applied")
	errResync = errors.New("revision gap")
)

// Blobs reads file contents from the local blob cache.
type Blobs interface {
	// Get returns the content for key with the expected hash.
	Get(ctx context.Context, key string, expected s3cache.Hash) ([]byte, s3cache.Hash, error)
}

// Config configures a remote file system.
type Config struct {
	// Cache configures the local paged B-tree caches.
	Cache CacheConfig
}

// RemoteFS is a read-only cached view of the files service.
type RemoteFS struct {
	logger *slog.Logger
	client filespb.FilesClient
	blobs  Blobs

	cache    *cache
	revision atomic.Uint64
	roots    sync.Map
}

// NewRemoteFS creates a remote file system.
func NewRemoteFS(config Config, client filespb.FilesClient, blobs Blobs, logger *slog.Logger) (*RemoteFS, error) {
	if client == nil || blobs == nil {
		return nil, ErrInvalidConfig
	}

	cache, err := newCache(config.Cache)
	if err != nil {
		return nil, err
	}

	return &RemoteFS{
		logger: logger,
		client: client,
		blobs:  blobs,
		cache:  cache,
	}, nil
}

// Stat implements files.FS.
func (fs *RemoteFS) Stat(ctx context.Context, path string) (files.Inode, error) {
	uid, err := uidFromContext(ctx)
	if err != nil {
		return files.Inode{}, err
	}

	inode, err := fs.resolve(uid, path)
	if err != nil {
		return files.Inode{}, err
	}

	return toInode(inode), nil
}

// List implements files.FS.
func (fs *RemoteFS) List(ctx context.Context, path string, cursor *files.Cursor) (files.Page, error) {
	uid, err := uidFromContext(ctx)
	if err != nil {
		return files.Page{}, err
	}

	inode, err := fs.resolve(uid, path)
	if err != nil {
		return files.Page{}, err
	}

	if !isDirectory(inode) {
		return files.Page{}, files.ErrNotDirectory
	}

	prefix := direntPrefix(uid, inode.GetIdentifier().GetNodeId())

	from := prefix
	if cursor != nil {
		from = prefix + string(*cursor) + "\x00"
	}

	inodes := []files.Inode{}
	limit := DefaultPageSize

	for key, data := range fs.cache.dirents.From(from) {
		if !strings.HasPrefix(key, prefix) {
			break
		}

		nodeID, err := decodeNodeID(data)
		if err != nil {
			return files.Page{}, err
		}

		dirent, ok, err := fs.cache.inode(uid, nodeID)
		if err != nil {
			return files.Page{}, err
		}

		if !ok {
			continue
		}

		inodes = append(inodes, toInode(dirent))

		if len(inodes) > limit {
			break
		}
	}

	page := files.Page{Inodes: inodes}

	if len(inodes) > limit {
		page.Inodes = inodes[:limit]
		next := files.Cursor(page.Inodes[limit-1].Name)
		page.Cursor = &next
	}

	return page, nil
}

// Open implements files.FS.
func (fs *RemoteFS) Open(ctx context.Context, path string) (files.File, error) {
	uid, err := uidFromContext(ctx)
	if err != nil {
		return nil, err
	}

	inode, err := fs.resolve(uid, path)
	if err != nil {
		return nil, err
	}

	if isDirectory(inode) {
		return nil, files.ErrIsDirectory
	}

	file := inode.GetFile()
	if file == nil {
		return nil, files.ErrNotFound
	}

	data, _, err := fs.blobs.Get(ctx, file.GetRef(), s3cache.Hash(string(file.GetHash())))
	if err != nil {
		return nil, fmt.Errorf("could not read blob for %q: %w", path, err)
	}

	return &openFile{
		Reader: bytes.NewReader(data),
		inode:  toInode(inode),
	}, nil
}

// Revision implements files.FS.
func (fs *RemoteFS) Revision() uint64 {
	return fs.revision.Load()
}

// Run implements run.Runnable.
func (fs *RemoteFS) Run(ctx context.Context, notify run.Notify) error {
	return run.Combine(
		&treeRunner{tree: fs.cache.inodes, ready: fs.cache.inodesReady},
		&treeRunner{tree: fs.cache.dirents, ready: fs.cache.direntsReady},
		&synchronizer{fs: fs},
	).Run(ctx, notify)
}

type synchronizer struct {
	fs *RemoteFS
}

// Run implements run.Runnable.
func (s *synchronizer) Run(ctx context.Context, notify run.Notify) error {
	if !s.fs.cache.wait(ctx) {
		return nil
	}

	if err := s.fs.snapshot(ctx); err != nil {
		return err
	}

	notify.Notify()

	s.fs.watch(ctx)

	return nil
}

func (fs *RemoteFS) snapshot(ctx context.Context) error {
	stream, err := fs.client.Snapshot(ctx, &filespb.SnapshotRequest{})
	if err != nil {
		return fmt.Errorf("could not start snapshot: %w", err)
	}

	if err := fs.cache.clear(); err != nil {
		return err
	}

	fs.roots.Clear()

	var revision uint64

	for {
		page, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return fmt.Errorf("could not receive snapshot: %w", err)
		}

		for _, inode := range page.GetInodes() {
			if err := fs.cache.put(inode); err != nil {
				return err
			}

			fs.trackRoot(inode)
		}

		revision = page.GetRevision()
	}

	fs.revision.Store(revision)

	return nil
}

func (fs *RemoteFS) watch(ctx context.Context) {
	backoffPolicy := backoff.NewExponentialBackOff()

	for {
		err := fs.consume(ctx)
		if ctx.Err() != nil {
			return
		}

		switch {
		case errors.Is(err, errResync):
			fs.logger.InfoContext(ctx, "resynchronizing remote files", "err", err)

			if err := fs.snapshot(ctx); err != nil {
				fs.logger.WarnContext(ctx, "remote files resync failed", "err", err)
			} else {
				backoffPolicy.Reset()

				continue
			}
		case err != nil:
			fs.logger.WarnContext(ctx, "files watch failed", "err", err)
		default:
			backoffPolicy.Reset()
		}

		timer := time.NewTimer(backoffPolicy.NextBackOff())

		select {
		case <-ctx.Done():
			timer.Stop()

			return
		case <-timer.C:
		}
	}
}

func (fs *RemoteFS) consume(ctx context.Context) error {
	revision := fs.revision.Load()

	stream, err := fs.client.Watch(ctx, &filespb.WatchRequest{AfterRevision: revision})
	if err != nil {
		return fmt.Errorf("could not start watch: %w", err)
	}

	for {
		event, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}

		if err != nil {
			if isRevisionTooOld(err) {
				return errResync
			}

			return fmt.Errorf("could not receive watch event: %w", err)
		}

		if err := fs.apply(event.GetEvent()); err != nil {
			if errors.Is(err, errSkip) {
				continue
			}

			return err
		}
	}
}

func (fs *RemoteFS) apply(event *filespb.Event) error {
	switch change := event.GetEvent().(type) {
	case *filespb.Event_Update:
		// The event carries the new state of an inode.
		return fs.applyUpdate(change.Update.GetInode())
	case *filespb.Event_Delete:
		// The event carries only the identifier of the removed inode.
		return fs.applyDelete(change.Delete)
	default:
		return ErrInvalidEvent
	}
}

func (fs *RemoteFS) applyUpdate(inode *filespb.Inode) error {
	if inode == nil || inode.GetIdentifier() == nil {
		return ErrInvalidInode
	}

	if err := fs.checkRevision(inode.GetRevision()); err != nil {
		return err
	}

	if err := fs.cache.put(inode); err != nil {
		return err
	}

	fs.trackRoot(inode)
	fs.revision.Store(inode.GetRevision())

	return nil
}

func (fs *RemoteFS) applyDelete(del *filespb.Delete) error {
	if del == nil || del.GetIdentifier() == nil {
		return ErrInvalidInode
	}

	if err := fs.checkRevision(del.GetRevision()); err != nil {
		return err
	}

	if _, err := fs.cache.delete(del.GetIdentifier().GetUid(), del.GetIdentifier().GetNodeId()); err != nil {
		return err
	}

	fs.revision.Store(del.GetRevision())

	return nil
}

func (fs *RemoteFS) checkRevision(revision uint64) error {
	current := fs.revision.Load()

	if revision <= current {
		return errSkip
	}

	if revision > current+1 {
		return errResync
	}

	return nil
}

func (fs *RemoteFS) resolve(uid int64, path string) (*filespb.Inode, error) {
	clean, err := files.CleanPath(path)
	if err != nil {
		return nil, err
	}

	root, ok := fs.root(uid)
	if !ok {
		return nil, files.ErrNotFound
	}

	current, ok, err := fs.cache.inode(uid, root)
	if err != nil {
		return nil, err
	}

	if !ok {
		return nil, files.ErrNotFound
	}

	if clean == string(files.PathSeparator) {
		return current, nil
	}

	names := strings.Split(strings.TrimPrefix(clean, string(files.PathSeparator)), string(files.PathSeparator))

	for i, name := range names {
		if !isDirectory(current) {
			return nil, files.ErrNotDirectory
		}

		nodeID, ok, err := fs.cache.dirent(uid, current.GetIdentifier().GetNodeId(), name)
		if err != nil {
			return nil, err
		}

		if !ok {
			return nil, files.ErrNotFound
		}

		current, ok, err = fs.cache.inode(uid, nodeID)
		if err != nil {
			return nil, err
		}

		if !ok {
			return nil, files.ErrNotFound
		}

		if i == len(names)-1 {
			return current, nil
		}
	}

	return current, nil
}

func (fs *RemoteFS) root(uid int64) (uint64, bool) {
	value, ok := fs.roots.Load(uid)
	if !ok {
		return 0, false
	}

	// The roots map stores uint64 values only.
	root, ok := value.(uint64)

	return root, ok
}

type uidContextKey struct{}

// ContextWithUID returns a context that selects the user tree for path lookups.
func ContextWithUID(ctx context.Context, uid int64) context.Context {
	return context.WithValue(ctx, uidContextKey{}, uid)
}

func uidFromContext(ctx context.Context) (int64, error) {
	uid, ok := ctx.Value(uidContextKey{}).(int64)
	if !ok {
		return 0, ErrNoUID
	}

	return uid, nil
}

func (fs *RemoteFS) trackRoot(inode *filespb.Inode) {
	identifier := inode.GetIdentifier()
	if identifier != nil && identifier.GetNodeId() == inode.GetParentId() {
		fs.roots.Store(identifier.GetUid(), identifier.GetNodeId())
	}
}

func isDirectory(inode *filespb.Inode) bool {
	return inode.GetDirectory() != nil
}

func isRevisionTooOld(err error) bool {
	if status.Code(err) == codes.OutOfRange {
		return true
	}

	return strings.Contains(status.Convert(err).Message(), RevisionTooOld)
}

func toInode(inode *filespb.Inode) files.Inode {
	out := files.Inode{
		ParentID: inode.GetParentId(),
		Name:     inode.GetName(),
		Revision: inode.GetRevision(),
	}

	if identifier := inode.GetIdentifier(); identifier != nil {
		out.UID = identifier.GetUid()
		out.NodeID = identifier.GetNodeId()
	}

	if file := inode.GetFile(); file != nil {
		out.Kind = files.KindFile
		out.Size = file.GetSize()
		out.Hash = files.Hash(string(file.GetHash()))
		out.S3Key = file.GetRef()
	}

	if modified := inode.GetModifiedAt(); modified != nil {
		out.ModTime = modified.AsTime()
	}

	return out
}

var (
	_ files.FS     = (*RemoteFS)(nil)
	_ run.Runnable = (*RemoteFS)(nil)
	_ Blobs        = (*s3cache.S3Cache)(nil)
)
