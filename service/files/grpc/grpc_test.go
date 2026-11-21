package grpc

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"slices"
	"strings"
	"testing"
	"time"

	grpcpkg "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	filespb "github.com/teapotovh/teapot/proto/files"
	"github.com/teapotovh/teapot/service/files/store"
)

func TestRevisionTooOld(t *testing.T) {
	t.Parallel()

	st, ctx := newMemoryStore(t)
	fs := st.For(1)

	if err := fs.Mkdir(ctx, "/a"); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	if err := fs.Put(ctx, "/a/f", strings.NewReader("hello")); err != nil {
		t.Fatalf("Put: %v", err)
	}

	stream := &watchStream{ctx: ctx}

	err := NewServer(filesStore{st}).Watch(&filespb.WatchRequest{
		AfterRevision: st.Revision() - 1,
	}, stream)
	if status.Code(err) != codes.OutOfRange {
		t.Fatalf("Watch code = %v, want %v", status.Code(err), codes.OutOfRange)
	}
}

func TestServerEndToEnd(t *testing.T) {
	t.Parallel()

	st, ctx := newMemoryStore(t)
	fs := st.For(4242)

	if err := fs.Mkdir(ctx, "/a"); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	if err := fs.Put(ctx, "/a/f", strings.NewReader("hello")); err != nil {
		t.Fatalf("Put: %v", err)
	}

	listener := bufconn.Listen(1 << 20)
	server := grpcpkg.NewServer()
	filespb.RegisterFilesServer(server, NewServer(filesStore{st}))

	go func() {
		if err := server.Serve(listener); err != nil {
			t.Logf("gRPC server stopped: %v", err)
		}
	}()

	conn, err := grpcpkg.NewClient(
		"passthrough:///bufconn",
		grpcpkg.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
		grpcpkg.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	t.Cleanup(func() {
		_ = conn.Close()

		server.Stop()

		_ = listener.Close()
	})

	client := filespb.NewFilesClient(conn)

	stream, err := client.Snapshot(ctx, &filespb.SnapshotRequest{})
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	names := []string{}

	var revision uint64

	for {
		page, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			t.Fatalf("Snapshot Recv: %v", err)
		}

		revision = page.GetRevision()

		for _, inode := range page.GetInodes() {
			names = append(names, inode.GetName())
		}
	}

	if revision == 0 {
		t.Fatal("snapshot revision = 0, want a positive value")
	}

	if !slices.Contains(names, "f") {
		t.Fatalf("snapshot names = %v, want f", names)
	}

	watchCtx, watchCancel := context.WithCancel(ctx)
	defer watchCancel()

	watch, err := client.Watch(watchCtx, &filespb.WatchRequest{AfterRevision: st.Revision()})
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	if err := fs.Put(ctx, "/a/g", strings.NewReader("world")); err != nil {
		t.Fatalf("Put: %v", err)
	}

	event, err := watch.Recv()
	if err != nil {
		t.Fatalf("Watch Recv: %v", err)
	}

	inode := event.GetEvent().GetUpdate().GetInode()
	if inode.GetName() != "g" {
		t.Fatalf("watch event name = %q, want g", inode.GetName())
	}
}

func newMemoryStore(t *testing.T) (*store.Store, context.Context) {
	t.Helper()

	st, err := store.NewMem(slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("NewMem: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	ready := make(chan struct{}, 1)

	go func() {
		done <- st.Run(ctx, notifyChannel(ready))
	}()

	select {
	case <-ready:
	case err := <-done:
		cancel()
		t.Fatalf("store Run: %v", err)
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("store did not start")
	}

	t.Cleanup(func() {
		cancel()

		if err := <-done; err != nil {
			t.Errorf("store Run: %v", err)
		}
	})

	return st, ctx
}

type filesStore struct {
	st *store.Store
}

// Store implements Files.
func (f filesStore) Store() *store.Store {
	return f.st
}

type notifyChannel chan struct{}

// Notify implements run.Notify.
func (n notifyChannel) Notify() {
	select {
	case n <- struct{}{}:
	default:
	}
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

type watchStream struct {
	stream
}

func (s *watchStream) Send(*filespb.WatchEvent) error {
	return nil
}
