package grpc

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/teapotovh/teapot/lib/files"
	filespb "github.com/teapotovh/teapot/proto/files"
	"github.com/teapotovh/teapot/service/files/store"
)

var errTestWatch = errors.New("watch failed")

func TestSnapshot(t *testing.T) {
	t.Parallel()

	source := &fakeStore{
		snapshot: func(_ context.Context, _ uint64, send func(uint64, []store.Inode) error) error {
			return send(7, []store.Inode{{
				UID:      42,
				NodeID:   3,
				ParentID: 2,
				Name:     "b",
				Kind:     files.KindFile,
				Size:     5,
				Hash:     files.Hash("etag"),
				S3Key:    "blob",
				Revision: 7,
			}})
		},
	}

	stream := &snapshotStream{ctx: context.Background()}

	if err := NewServer(source).Snapshot(&filespb.SnapshotRequest{}, stream); err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}

	if len(stream.pages) != 1 {
		t.Fatalf("Snapshot() sent %d pages, want 1", len(stream.pages))
	}

	page := stream.pages[0]
	if page.GetRevision() != 7 {
		t.Fatalf("snapshot revision = %d, want 7", page.GetRevision())
	}

	if len(page.GetInodes()) != 1 {
		t.Fatalf("snapshot inodes = %d, want 1", len(page.GetInodes()))
	}

	inode := page.GetInodes()[0]
	if inode.GetIdentifier().GetNodeId() != 3 || inode.GetName() != "b" || inode.GetFile().GetRef() != "blob" {
		t.Fatalf("snapshot inode = %#v, want inode b", inode)
	}
}

func TestWatch(t *testing.T) {
	t.Parallel()

	source := &fakeStore{
		watch: func(_ context.Context, _ uint64, send func(store.Event) error) error {
			return send(store.Event{Inode: store.Inode{
				UID:      42,
				NodeID:   3,
				ParentID: 2,
				Name:     "b",
				Kind:     files.KindDirectory,
				Revision: 8,
			}})
		},
	}

	stream := &watchStream{ctx: context.Background()}

	if err := NewServer(source).Watch(&filespb.WatchRequest{}, stream); err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	if len(stream.events) != 1 {
		t.Fatalf("Watch() sent %d events, want 1", len(stream.events))
	}

	inode := stream.events[0].GetEvent().GetUpdate().GetInode()
	if inode.GetIdentifier().GetNodeId() != 3 || inode.GetName() != "b" || inode.GetRevision() != 8 {
		t.Fatalf("watch inode = %#v, want inode b at revision 8", inode)
	}
}

func TestRevisionTooOld(t *testing.T) {
	t.Parallel()

	source := &fakeStore{
		snapshot: func(context.Context, uint64, func(uint64, []store.Inode) error) error {
			return store.ErrRevisionTooOld
		},
	}

	stream := &snapshotStream{ctx: context.Background()}

	err := NewServer(source).Snapshot(&filespb.SnapshotRequest{}, stream)
	if status.Code(err) != codes.OutOfRange {
		t.Fatalf("Snapshot() code = %v, want %v", status.Code(err), codes.OutOfRange)
	}
}

func TestInternalError(t *testing.T) {
	t.Parallel()

	source := &fakeStore{
		watch: func(context.Context, uint64, func(store.Event) error) error {
			return errTestWatch
		},
	}

	stream := &watchStream{ctx: context.Background()}

	err := NewServer(source).Watch(&filespb.WatchRequest{}, stream)
	if status.Code(err) != codes.Internal {
		t.Fatalf("Watch() code = %v, want %v", status.Code(err), codes.Internal)
	}
}

type fakeStore struct {
	snapshot func(context.Context, uint64, func(uint64, []store.Inode) error) error
	watch    func(context.Context, uint64, func(store.Event) error) error
}

// Snapshot implements Store.
func (s *fakeStore) Snapshot(
	ctx context.Context,
	after uint64,
	send func(uint64, []store.Inode) error,
) error {
	return s.snapshot(ctx, after, send)
}

// Watch implements Store.
func (s *fakeStore) Watch(ctx context.Context, after uint64, send func(store.Event) error) error {
	return s.watch(ctx, after, send)
}

type stream struct {
	ctx context.Context
}

func (s *stream) SetHeader(metadata.MD) error  { return nil }
func (s *stream) SendHeader(metadata.MD) error { return nil }
func (s *stream) SetTrailer(metadata.MD)       {}
func (s *stream) Context() context.Context     { return s.ctx }
func (s *stream) SendMsg(any) error            { return nil }
func (s *stream) RecvMsg(any) error            { return nil }

type snapshotStream struct {
	stream

	pages []*filespb.SnapshotPage
}

func (s *snapshotStream) Send(page *filespb.SnapshotPage) error {
	s.pages = append(s.pages, page)

	return nil
}

type watchStream struct {
	stream

	events []*filespb.WatchEvent
}

func (s *watchStream) Send(event *filespb.WatchEvent) error {
	s.events = append(s.events, event)

	return nil
}

var _ Store = (*fakeStore)(nil)
