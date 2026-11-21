package remotefiles

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/teapotovh/teapot/lib/files"
	"github.com/teapotovh/teapot/lib/s3cache"
	filespb "github.com/teapotovh/teapot/proto/files"
)

const testUID = 42

func testContext() context.Context {
	return ContextWithUID(context.Background(), testUID)
}

func TestSnapshotAndReads(t *testing.T) {
	t.Parallel()

	client := &fakeClient{
		pages: []*filespb.SnapshotPage{
			{
				Revision: 2,
				Inodes: []*filespb.Inode{
					directory(testUID, 1, 1, "", 1),
					directory(testUID, 2, 1, "a", 2),
					directory(99, 50, 50, "", 2),
				},
			},
			{
				Revision: 3,
				Inodes: []*filespb.Inode{
					file(testUID, 3, 2, "b", 3, "blob-b", "etag-b", "hello"),
					file(testUID, 4, 1, "z", 3, "blob-z", "etag-z", "world"),
				},
			},
		},
		events: make(chan *filespb.WatchEvent),
	}

	fs := startRemoteFS(t, client, fakeBlobs{
		"blob-b": []byte("hello"),
		"blob-z": []byte("world"),
	})

	waitFor(t, func() bool {
		_, errB := fs.Stat(testContext(), "/a/b")
		_, errZ := fs.Stat(testContext(), "/z")

		return errB == nil && errZ == nil
	})

	if got := fs.Revision(); got != 3 {
		t.Fatalf("Revision() = %d, want 3", got)
	}

	stat, err := fs.Stat(testContext(), "/")
	if err != nil {
		t.Fatal(err)
	}

	if stat.NodeID != 1 || stat.Kind != files.KindDirectory {
		t.Fatalf("Stat(/) = %#v, want root directory", stat)
	}

	other, err := fs.Stat(ContextWithUID(context.Background(), 99), "/")
	if err != nil {
		t.Fatal(err)
	}

	if other.NodeID != 50 || other.UID != 99 {
		t.Fatalf("Stat(/) for uid 99 = %#v, want root inode 50", other)
	}

	stat, err = fs.Stat(testContext(), "/a/b")
	if err != nil {
		t.Fatal(err)
	}

	if stat.NodeID != 3 || stat.Kind != files.KindFile || stat.Size != int64(len("hello")) ||
		stat.S3Key != "blob-b" || stat.Hash != files.Hash("etag-b") {
		t.Fatalf("Stat(/a/b) = %#v, want file b", stat)
	}

	page, err := fs.List(testContext(), "/", nil)
	if err != nil {
		t.Fatal(err)
	}

	if len(page.Inodes) != 2 || page.Inodes[0].Name != "a" || page.Inodes[1].Name != "z" || page.Cursor != nil {
		t.Fatalf("List(/) = %#v, want [a z] without cursor", page)
	}

	opened, err := fs.Open(testContext(), "/a/b")
	if err != nil {
		t.Fatal(err)
	}

	stat, err = opened.Stat()
	if err != nil {
		t.Fatal(err)
	}

	if stat.NodeID != 3 || stat.Name != "b" {
		t.Fatalf("File.Stat() = %#v, want file b", stat)
	}

	data, err := io.ReadAll(opened)
	if err != nil {
		t.Fatal(err)
	}

	if string(data) != "hello" {
		t.Fatalf("Open(/a/b) = %q, want %q", data, "hello")
	}

	if offset, err := opened.Seek(0, io.SeekStart); err != nil || offset != 0 {
		t.Fatalf("File.Seek(0) = %d, %v, want 0, nil", offset, err)
	}

	if err := opened.Close(); err != nil {
		t.Fatalf("File.Close() error = %v", err)
	}
}

func TestWatchUpdateAndDelete(t *testing.T) {
	t.Parallel()

	client := &fakeClient{
		pages: []*filespb.SnapshotPage{{
			Revision: 2,
			Inodes: []*filespb.Inode{
				directory(testUID, 1, 1, "", 1),
				file(testUID, 2, 1, "old", 2, "blob", "etag", "data"),
			},
		}},
		events: make(chan *filespb.WatchEvent),
	}

	fs := startRemoteFS(t, client, fakeBlobs{"blob": []byte("data")})

	waitFor(t, func() bool {
		_, err := fs.Stat(testContext(), "/old")

		return err == nil
	})

	client.events <- &filespb.WatchEvent{
		Event: &filespb.Event{
			Event: &filespb.Event_Update{
				Update: &filespb.Update{
					Inode: file(testUID, 2, 1, "new", 3, "blob", "etag", "data"),
				},
			},
		},
	}

	waitFor(t, func() bool {
		_, err := fs.Stat(testContext(), "/new")

		return err == nil
	})

	if _, err := fs.Stat(testContext(), "/old"); !errors.Is(err, files.ErrNotFound) {
		t.Fatalf("Stat(/old) error = %v, want %v", err, files.ErrNotFound)
	}

	client.events <- &filespb.WatchEvent{
		Event: &filespb.Event{
			Event: &filespb.Event_Delete{
				Delete: &filespb.Delete{
					Identifier: &filespb.InodeIdentifier{Uid: testUID, NodeId: 2},
					Revision:   4,
				},
			},
		},
	}

	waitFor(t, func() bool {
		_, err := fs.Stat(testContext(), "/new")

		return errors.Is(err, files.ErrNotFound)
	})

	if got := fs.Revision(); got != 4 {
		t.Fatalf("Revision() = %d, want 4", got)
	}
}

func TestReadErrors(t *testing.T) {
	t.Parallel()

	client := &fakeClient{
		pages: []*filespb.SnapshotPage{{
			Revision: 1,
			Inodes: []*filespb.Inode{
				directory(testUID, 1, 1, "", 1),
				file(testUID, 2, 1, "file", 1, "blob", "etag", "data"),
			},
		}},
		events: make(chan *filespb.WatchEvent),
	}

	fs := startRemoteFS(t, client, fakeBlobs{})

	waitFor(t, func() bool {
		_, err := fs.Stat(testContext(), "/file")

		return err == nil
	})

	if _, err := fs.List(testContext(), "/file", nil); !errors.Is(err, files.ErrNotDirectory) {
		t.Fatalf("List(file) error = %v, want %v", err, files.ErrNotDirectory)
	}

	if _, err := fs.Open(testContext(), "/"); !errors.Is(err, files.ErrIsDirectory) {
		t.Fatalf("Open(/) error = %v, want %v", err, files.ErrIsDirectory)
	}

	if _, err := fs.List(testContext(), "/missing", nil); !errors.Is(err, files.ErrNotFound) {
		t.Fatalf("List(missing) error = %v, want %v", err, files.ErrNotFound)
	}

	if _, err := fs.Open(testContext(), "/file"); err == nil {
		t.Fatal("Open(file) error = nil, want a blob error")
	}

	if _, err := fs.Stat(testContext(), "/file/child"); !errors.Is(err, files.ErrNotDirectory) {
		t.Fatalf("Stat(file/child) error = %v, want %v", err, files.ErrNotDirectory)
	}

	if _, err := fs.Stat(testContext(), "/missing"); !errors.Is(err, files.ErrNotFound) {
		t.Fatalf("Stat(missing) error = %v, want %v", err, files.ErrNotFound)
	}

	if _, err := fs.Stat(testContext(), "relative"); !errors.Is(err, files.ErrInvalidPath) {
		t.Fatalf("Stat(relative) error = %v, want %v", err, files.ErrInvalidPath)
	}
}

func TestFlags(t *testing.T) {
	t.Parallel()

	flags, config := RemoteFilesFlagSet()

	err := flags.Parse([]string{
		"--remotefiles-inodes-path=/tmp/inodes",
		"--remotefiles-inodes-memory=1024",
		"--remotefiles-inodes-disk=2048",
		"--remotefiles-dirents-path=/tmp/dirents",
		"--remotefiles-dirents-memory=4096",
		"--remotefiles-dirents-disk=8192",
	})
	if err != nil {
		t.Fatal(err)
	}

	got := config()
	want := Config{
		Cache: CacheConfig{
			Inodes: TreeConfig{
				Path:   "/tmp/inodes",
				Memory: 1024,
				Disk:   2048,
			},
			Dirents: TreeConfig{
				Path:   "/tmp/dirents",
				Memory: 4096,
				Disk:   8192,
			},
		},
	}

	if got != want {
		t.Fatalf("config() = %#v, want %#v", got, want)
	}
}

func TestMissingUID(t *testing.T) {
	t.Parallel()

	fs := &RemoteFS{}

	if _, err := fs.Stat(context.Background(), "/"); !errors.Is(err, ErrNoUID) {
		t.Fatalf("Stat() error = %v, want %v", err, ErrNoUID)
	}

	if _, err := fs.List(context.Background(), "/", nil); !errors.Is(err, ErrNoUID) {
		t.Fatalf("List() error = %v, want %v", err, ErrNoUID)
	}

	if _, err := fs.Open(context.Background(), "/"); !errors.Is(err, ErrNoUID) {
		t.Fatalf("Open() error = %v, want %v", err, ErrNoUID)
	}
}

func TestRevisionRules(t *testing.T) {
	t.Parallel()

	fs := &RemoteFS{}
	fs.revision.Store(5)

	if err := fs.checkRevision(4); !errors.Is(err, errSkip) {
		t.Fatalf("checkRevision(4) error = %v, want %v", err, errSkip)
	}

	if err := fs.checkRevision(5); !errors.Is(err, errSkip) {
		t.Fatalf("checkRevision(5) error = %v, want %v", err, errSkip)
	}

	if err := fs.checkRevision(6); err != nil {
		t.Fatalf("checkRevision(6) error = %v, want nil", err)
	}

	if err := fs.checkRevision(7); !errors.Is(err, errResync) {
		t.Fatalf("checkRevision(7) error = %v, want %v", err, errResync)
	}

	if !isRevisionTooOld(status.Error(codes.OutOfRange, "out of range")) {
		t.Fatal("isRevisionTooOld(OutOfRange) = false, want true")
	}

	if !isRevisionTooOld(status.Error(codes.Internal, RevisionTooOld)) {
		t.Fatal("isRevisionTooOld(REVISION_TOO_OLD) = false, want true")
	}

	if isRevisionTooOld(status.Error(codes.Internal, "other")) {
		t.Fatal("isRevisionTooOld(other) = true, want false")
	}
}

func TestInvalidEvents(t *testing.T) {
	t.Parallel()

	fs := &RemoteFS{}
	fs.revision.Store(5)

	if err := fs.apply(&filespb.Event{}); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("apply(empty) error = %v, want %v", err, ErrInvalidEvent)
	}

	if err := fs.applyUpdate(nil); !errors.Is(err, ErrInvalidInode) {
		t.Fatalf("applyUpdate(nil) error = %v, want %v", err, ErrInvalidInode)
	}

	if err := fs.applyUpdate(&filespb.Inode{Revision: 6}); !errors.Is(err, ErrInvalidInode) {
		t.Fatalf("applyUpdate(no identifier) error = %v, want %v", err, ErrInvalidInode)
	}

	if err := fs.applyDelete(nil); !errors.Is(err, ErrInvalidInode) {
		t.Fatalf("applyDelete(nil) error = %v, want %v", err, ErrInvalidInode)
	}

	if err := fs.applyDelete(&filespb.Delete{Revision: 6}); !errors.Is(err, ErrInvalidInode) {
		t.Fatalf("applyDelete(no identifier) error = %v, want %v", err, ErrInvalidInode)
	}

	if err := fs.applyUpdate(&filespb.Inode{
		Identifier: &filespb.InodeIdentifier{Uid: testUID, NodeId: 1},
		Revision:   5,
	}); !errors.Is(err, errSkip) {
		t.Fatalf("applyUpdate(old revision) error = %v, want %v", err, errSkip)
	}

	if err := fs.applyDelete(&filespb.Delete{
		Identifier: &filespb.InodeIdentifier{Uid: testUID, NodeId: 1},
		Revision:   5,
	}); !errors.Is(err, errSkip) {
		t.Fatalf("applyDelete(old revision) error = %v, want %v", err, errSkip)
	}

	if err := fs.applyUpdate(&filespb.Inode{
		Identifier: &filespb.InodeIdentifier{Uid: testUID, NodeId: 1},
		Revision:   7,
	}); !errors.Is(err, errResync) {
		t.Fatalf("applyUpdate(gap) error = %v, want %v", err, errResync)
	}
}

func startRemoteFS(t *testing.T, client filespb.FilesClient, blobs Blobs) *RemoteFS {
	t.Helper()

	fs, err := NewRemoteFS(Config{
		Cache: CacheConfig{
			Inodes: TreeConfig{
				Path:   filepath.Join(t.TempDir(), "inodes"),
				Memory: 1 << 20,
			},
			Dirents: TreeConfig{
				Path:   filepath.Join(t.TempDir(), "dirents"),
				Memory: 1 << 20,
			},
		},
	}, client, blobs, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(testContext())
	ready := make(chan struct{}, 1)
	runDone := make(chan error, 1)

	go func() {
		runDone <- fs.Run(ctx, notifyChannel(ready))
	}()

	t.Cleanup(func() {
		cancel()

		select {
		case err := <-runDone:
			if err != nil {
				t.Errorf("Run() error = %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("Run() did not stop")
		}
	})

	select {
	case <-ready:
	case err := <-runDone:
		t.Fatalf("Run() stopped before start: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not start")
	}

	return fs
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("condition was not met before timeout")
		}

		time.Sleep(10 * time.Millisecond)
	}
}

type notifyChannel chan struct{}

// Notify implements run.Notify.
func (n notifyChannel) Notify() {
	select {
	case n <- struct{}{}:
	default:
	}
}

type fakeClient struct {
	pages  []*filespb.SnapshotPage
	events chan *filespb.WatchEvent
}

// Snapshot implements files.FilesClient.
func (c *fakeClient) Snapshot(
	ctx context.Context,
	_ *filespb.SnapshotRequest,
	_ ...grpc.CallOption,
) (filespb.Files_SnapshotClient, error) {
	return &snapshotStream{
		ctx:   ctx,
		pages: c.pages,
	}, nil
}

// Watch implements files.FilesClient.
func (c *fakeClient) Watch(
	ctx context.Context,
	_ *filespb.WatchRequest,
	_ ...grpc.CallOption,
) (filespb.Files_WatchClient, error) {
	return &watchStream{
		ctx:    ctx,
		events: c.events,
	}, nil
}

type stream struct {
	ctx context.Context
}

// Header implements grpc.ClientStream.
func (stream) Header() (metadata.MD, error) { return metadata.MD{}, nil }

// Trailer implements grpc.ClientStream.
func (stream) Trailer() metadata.MD { return nil }

// CloseSend implements grpc.ClientStream.
func (stream) CloseSend() error { return nil }

// Context implements grpc.ClientStream.
func (s stream) Context() context.Context { return s.ctx }

// SendMsg implements grpc.ClientStream.
func (stream) SendMsg(any) error { return nil }

// RecvMsg implements grpc.ClientStream.
func (stream) RecvMsg(any) error { return io.EOF }

type snapshotStream struct {
	stream

	pages []*filespb.SnapshotPage
	index int
}

func (s *snapshotStream) Recv() (*filespb.SnapshotPage, error) {
	if s.index >= len(s.pages) {
		return nil, io.EOF
	}

	page := s.pages[s.index]
	s.index++

	return page, nil
}

type watchStream struct {
	stream

	events <-chan *filespb.WatchEvent
}

func (s *watchStream) Recv() (*filespb.WatchEvent, error) {
	select {
	case event := <-s.events:
		return event, nil
	case <-s.ctx.Done():
		return nil, io.EOF
	}
}

type fakeBlobs map[string][]byte

// Get implements Blobs.
func (b fakeBlobs) Get(_ context.Context, key string, expected s3cache.Hash) ([]byte, s3cache.Hash, error) {
	data, ok := b[key]
	if !ok {
		return nil, s3cache.ZeroHash, files.ErrNotFound
	}

	return data, expected, nil
}

func directory(uid int64, nodeID, parentID uint64, name string, revision uint64) *filespb.Inode {
	return &filespb.Inode{
		Identifier: &filespb.InodeIdentifier{Uid: uid, NodeId: nodeID},
		Revision:   revision,
		ParentId:   parentID,
		Name:       name,
		Kind:       &filespb.Inode_Directory{Directory: &filespb.DirectoryKind{}},
	}
}

func file(uid int64, nodeID, parentID uint64, name string, revision uint64, ref, hash, content string) *filespb.Inode {
	return &filespb.Inode{
		Identifier: &filespb.InodeIdentifier{Uid: uid, NodeId: nodeID},
		Revision:   revision,
		ParentId:   parentID,
		Name:       name,
		Kind: &filespb.Inode_File{File: &filespb.FileKind{
			Ref:  ref,
			Hash: []byte(hash),
			Size: int64(len(content)),
		}},
	}
}
