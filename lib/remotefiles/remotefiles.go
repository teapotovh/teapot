package remotefiles

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"github.com/teapotovh/teapot/lib/fs"
	"github.com/teapotovh/teapot/lib/run"
	"github.com/teapotovh/teapot/lib/s3cache"
	filespb "github.com/teapotovh/teapot/proto/files"
)

const (
	// defaultPageSize is the maximum number of inodes in one list page.
	defaultPageSize = 1000

	// MinimumRetryTime is the minimum time between full resyncs.
	MinimumRetryTime = 5 * time.Minute

	rootNodeID uint64 = 1
)

var (
	// ErrInvalidConfig reports a missing or invalid configuration.
	ErrInvalidConfig = errors.New("invalid config")
	// ErrInvalidInode reports an inode without an identifier.
	ErrInvalidInode = errors.New("invalid inode")
	// ErrInvalidEvent reports an event without a valid update or delete.
	ErrInvalidEvent = errors.New("invalid event")
	// ErrRetriedTooFast reports a full resync that follows the previous one too soon.
	ErrRetriedTooFast = errors.New("retried too fast")

	// ErrSkip reports an event that is not newer than the current revision.
	ErrSkip = errors.New("event already applied")
	// ErrResync reports a missing revision. A full snapshot is needed.
	ErrResync = errors.New("revision gap")
)

// Config configures a remote file system.
type Config struct {
	// Cache configures the local paged B-tree caches.
	Cache CacheConfig
}

// RemoteFiles is a read-only cached view of the files service.
type RemoteFiles struct {
	logger *slog.Logger
	client filespb.FilesClient
	blobs  s3cache.Blob

	cache    *cache
	revision atomic.Uint64
}

// NewRemoteFiles creates a remote file system.
func NewRemoteFiles(
	config Config,
	client filespb.FilesClient,
	blobs s3cache.Blob,
	logger *slog.Logger,
) (*RemoteFiles, error) {
	if client == nil || blobs == nil {
		return nil, ErrInvalidConfig
	}

	cache, err := newCache(config.Cache)
	if err != nil {
		return nil, fmt.Errorf("could not create remote files cache: %w", err)
	}

	return &RemoteFiles{
		logger: logger,
		client: client,
		blobs:  blobs,
		cache:  cache,
	}, nil
}

// For returns the file system view for one user.
func (r *RemoteFiles) For(uid int64) *RemoteFS {
	return &RemoteFS{files: r, uid: uid}
}

// RemoteFS is the read-only file system view for one user.
type RemoteFS struct {
	files *RemoteFiles
	uid   int64
}

// Stat implements FileSystem.
func (fs *RemoteFS) Stat(ctx context.Context, path string) (Inode, error) {
	inode, err := fs.files.resolve(fs.uid, path)
	if err != nil {
		return Inode{}, fmt.Errorf("could not resolve path %q: %w", path, err)
	}

	return toInode(inode), nil
}

// List implements FileSystem.
func (fs *RemoteFS) List(ctx context.Context, path string, cursor *Cursor) (Page, error) {
	inode, err := fs.files.resolve(fs.uid, path)
	if err != nil {
		return Page{}, fmt.Errorf("could not resolve path %q: %w", path, err)
	}

	if !isDirectory(inode) {
		return Page{}, ErrNotDirectory
	}

	prefix := direntPrefix(fs.uid, inode.GetIdentifier().GetNodeId())

	from := prefix
	if cursor != nil {
		from = prefix + string(*cursor) + "\x00"
	}

	inodes := []Inode{}
	limit := defaultPageSize

	for key, value := range fs.files.cache.dirents.From(from) {
		if !strings.HasPrefix(key, prefix) {
			break
		}

		dirent, ok, err := fs.files.cache.inode(fs.uid, value.nodeID)
		if err != nil {
			return Page{}, fmt.Errorf("could not read inode %d: %w", value.nodeID, err)
		}

		if !ok {
			continue
		}

		inodes = append(inodes, toInode(dirent))

		if len(inodes) > limit {
			break
		}
	}

	page := Page{Inodes: inodes}

	if len(inodes) > limit {
		page.Inodes = inodes[:limit]
		next := Cursor(page.Inodes[limit-1].Name)
		page.Cursor = &next
	}

	return page, nil
}

// Open implements FileSystem.
func (fs *RemoteFS) Open(ctx context.Context, path string) (File, error) {
	inode, err := fs.files.resolve(fs.uid, path)
	if err != nil {
		return nil, fmt.Errorf("could not resolve path %q: %w", path, err)
	}

	if isDirectory(inode) {
		return nil, ErrIsDirectory
	}

	file := inode.GetFile()
	if file == nil {
		return nil, ErrNotFound
	}

	reader, _, err := fs.files.blobs.Get(ctx, file.GetRef(), s3cache.Hash(string(file.GetHash())))
	if err != nil {
		return nil, fmt.Errorf("could not read blob for %q: %w", path, err)
	}

	return &openFile{
		ReadSeekCloser: reader,
		inode:          toInode(inode),
	}, nil
}

// Revision implements FileSystem.
func (fs *RemoteFS) Revision() uint64 {
	return fs.files.revision.Load()
}

// Run implements run.Runnable.
func (r *RemoteFiles) Run(step run.Step) error {
	step.Add("inodes", r.cache.inodes)
	step.Add("dirents", r.cache.dirents)
	step.Wait()

	ctx := step.Context()
	lastStartup := time.Unix(0, 0)
	lastErr := error(nil)

	for {
		if ctx.Err() != nil {
			return nil //nolint:nilerr // The context was canceled.
		}

		if time.Since(lastStartup) < MinimumRetryTime {
			return fmt.Errorf(
				"could not retry remote files synchronization: %w, last error was: %w",
				ErrRetriedTooFast,
				lastErr,
			)
		}

		lastStartup = time.Now()

		if err := r.process(ctx, step); err != nil {
			lastErr = err

			r.logger.WarnContext(ctx, "remote files synchronization failed, retrying", "err", err)
		}
	}
}

func (r *RemoteFiles) process(ctx context.Context, step run.Step) error {
	if err := r.snapshot(ctx); err != nil {
		return fmt.Errorf("could not snapshot remote files: %w", err)
	}

	step.Notify()

	if err := r.consume(ctx); err != nil {
		return fmt.Errorf("could not consume remote files events: %w", err)
	}

	return nil
}

func (r *RemoteFiles) snapshot(ctx context.Context) error {
	stream, err := r.client.Snapshot(ctx, &filespb.SnapshotRequest{})
	if err != nil {
		return fmt.Errorf("could not start snapshot: %w", err)
	}

	if err := r.cache.clear(); err != nil {
		return fmt.Errorf("could not clear remote files cache: %w", err)
	}

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
			if err := r.cache.put(inode); err != nil {
				return fmt.Errorf("could not store snapshot inode: %w", err)
			}
		}

		revision = page.GetRevision()
	}

	r.revision.Store(revision)

	return nil
}

func (r *RemoteFiles) consume(ctx context.Context) error {
	revision := r.revision.Load()

	stream, err := r.client.Watch(ctx, &filespb.WatchRequest{AfterRevision: revision})
	if err != nil {
		return fmt.Errorf("could not start watch: %w", err)
	}

	for {
		event, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}

		if err != nil {
			return fmt.Errorf("could not receive watch event: %w", err)
		}

		if err := r.apply(event.GetEvent()); err != nil {
			if errors.Is(err, ErrSkip) {
				continue
			}

			return fmt.Errorf("could not apply watch event: %w", err)
		}
	}
}

func (r *RemoteFiles) apply(event *filespb.Event) error {
	switch change := event.GetEvent().(type) {
	case *filespb.Event_Update:
		// The event carries the new state of an inode.
		return r.applyUpdate(change.Update.GetInode())
	case *filespb.Event_Delete:
		// The event carries only the identifier of the removed inode.
		return r.applyDelete(change.Delete)
	default:
		return ErrInvalidEvent
	}
}

func (r *RemoteFiles) applyUpdate(inode *filespb.Inode) error {
	if inode == nil || inode.GetIdentifier() == nil {
		return ErrInvalidInode
	}

	if err := r.checkRevision(inode.GetRevision()); err != nil {
		return fmt.Errorf("could not check inode revision: %w", err)
	}

	if err := r.cache.put(inode); err != nil {
		return fmt.Errorf("could not store inode: %w", err)
	}

	r.revision.Store(inode.GetRevision())

	return nil
}

func (r *RemoteFiles) applyDelete(del *filespb.Delete) error {
	if del == nil || del.GetIdentifier() == nil {
		return ErrInvalidInode
	}

	if err := r.checkRevision(del.GetRevision()); err != nil {
		return fmt.Errorf("could not check inode revision: %w", err)
	}

	if _, err := r.cache.delete(del.GetIdentifier().GetUid(), del.GetIdentifier().GetNodeId()); err != nil {
		return fmt.Errorf("could not delete inode: %w", err)
	}

	r.revision.Store(del.GetRevision())

	return nil
}

func (r *RemoteFiles) checkRevision(revision uint64) error {
	current := r.revision.Load()

	if revision <= current {
		return ErrSkip
	}

	if revision > current+1 {
		return ErrResync
	}

	return nil
}

func (r *RemoteFiles) resolve(uid int64, path string) (*filespb.Inode, error) {
	clean, err := fs.CleanPath(path)
	if err != nil {
		return nil, err
	}

	current, ok, err := r.cache.inode(uid, rootNodeID)
	if err != nil {
		return nil, fmt.Errorf("could not read root inode: %w", err)
	}

	if !ok {
		return nil, ErrNotFound
	}

	if clean == string(fs.PathSeparator) {
		return current, nil
	}

	names := strings.Split(strings.TrimPrefix(clean, string(fs.PathSeparator)), string(fs.PathSeparator))

	for i, name := range names {
		if !isDirectory(current) {
			return nil, ErrNotDirectory
		}

		nodeID, ok, err := r.cache.dirent(uid, current.GetIdentifier().GetNodeId(), name)
		if err != nil {
			return nil, fmt.Errorf("could not read directory entry %q: %w", name, err)
		}

		if !ok {
			return nil, ErrNotFound
		}

		current, ok, err = r.cache.inode(uid, nodeID)
		if err != nil {
			return nil, fmt.Errorf("could not read inode %d: %w", nodeID, err)
		}

		if !ok {
			return nil, ErrNotFound
		}

		if i == len(names)-1 {
			return current, nil
		}
	}

	return current, nil
}

func isDirectory(inode *filespb.Inode) bool {
	return inode.GetDirectory() != nil
}

func toInode(inode *filespb.Inode) Inode {
	out := Inode{
		ParentID: inode.GetParentId(),
		Name:     inode.GetName(),
		Revision: inode.GetRevision(),
	}

	if identifier := inode.GetIdentifier(); identifier != nil {
		out.UID = identifier.GetUid()
		out.NodeID = identifier.GetNodeId()
	}

	if file := inode.GetFile(); file != nil {
		out.Kind = KindFile
		out.Size = file.GetSize()
		out.Hash = Hash(string(file.GetHash()))
		out.S3Key = file.GetRef()
	}

	if modified := inode.GetModifiedAt(); modified != nil {
		out.ModTime = modified.AsTime()
	}

	return out
}

var (
	// Ensure RemoteFS implements FileSystem.
	_ FileSystem = (*RemoteFS)(nil)
	// Ensure RemoteFiles implements run.Runnable.
	_ run.Runnable = (*RemoteFiles)(nil)
	// Ensure S3Cache implements s3cache.Blob.
	_ s3cache.Blob = (*s3cache.S3Cache)(nil)
)
