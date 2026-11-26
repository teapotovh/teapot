package store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/teapotovh/teapot/lib/run"
)

func TestFSWriteRead(t *testing.T) {
	t.Parallel()

	store, ctx := newTestStore(t)
	fs := store.For(1000)

	if err := fs.Mkdir(ctx, "/a"); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	if err := fs.Put(ctx, "/a/f.txt", bytes.NewBufferString("hello")); err != nil {
		t.Fatalf("Put: %v", err)
	}

	inode, err := fs.Stat(ctx, "/a/f.txt")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}

	if inode.Size != 5 {
		t.Fatalf("Stat size = %d, want 5", inode.Size)
	}

	file, err := fs.Open(ctx, "/a/f.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	data, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if string(data) != "hello" {
		t.Fatalf("content = %q, want %q", data, "hello")
	}

	if err := file.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if err := fs.Rename(ctx, "/a/f.txt", "/a/g.txt"); err != nil {
		t.Fatalf("Rename: %v", err)
	}

	if _, err := fs.Stat(ctx, "/a/f.txt"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Stat old path error = %v, want not found", err)
	}

	if _, err := fs.Stat(ctx, "/a/g.txt"); err != nil {
		t.Fatalf("Stat new path: %v", err)
	}

	page, err := fs.List(ctx, "/a", nil)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(page.Inodes) != 1 || page.Inodes[0].Name != "g.txt" {
		t.Fatalf("List = %+v, want one g.txt", page.Inodes)
	}

	if err := fs.Delete(ctx, "/a/g.txt"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	page, err = fs.List(ctx, "/a", nil)
	if err != nil {
		t.Fatalf("List after delete: %v", err)
	}

	if len(page.Inodes) != 0 {
		t.Fatalf("List after delete = %+v, want empty", page.Inodes)
	}
}

func TestFSNameValidation(t *testing.T) {
	t.Parallel()

	store, ctx := newTestStore(t)
	fs := store.For(1001)

	if err := fs.Mkdir(ctx, "/a"); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	for _, path := range []string{"/a/b\x00c"} {
		if err := fs.Put(ctx, path, bytes.NewBufferString("x")); !errors.Is(err, ErrInvalidPath) {
			t.Fatalf("Put(%q) error = %v, want invalid path", path, err)
		}
	}

	for _, path := range []string{"/a/b\x01c"} {
		if err := fs.Put(ctx, path, bytes.NewBufferString("x")); !errors.Is(err, ErrInvalidName) {
			t.Fatalf("Put(%q) error = %v, want invalid name", path, err)
		}
	}

	for _, path := range []string{"/a/..", "/a/../b"} {
		if err := fs.Put(ctx, path, bytes.NewBufferString("x")); !errors.Is(err, ErrInvalidPath) {
			t.Fatalf("Put(%q) error = %v, want invalid path", path, err)
		}
	}
}

func TestFSDeleteNonEmptyDirectory(t *testing.T) {
	t.Parallel()

	store, ctx := newTestStore(t)
	fs := store.For(1002)

	if err := fs.Mkdir(ctx, "/a"); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	if err := fs.Put(ctx, "/a/f", bytes.NewBufferString("x")); err != nil {
		t.Fatalf("Put: %v", err)
	}

	if err := fs.Delete(ctx, "/a"); !errors.Is(err, ErrNotEmpty) {
		t.Fatalf("Delete error = %v, want ErrNotEmpty", err)
	}
}

func TestFSRenameCycle(t *testing.T) {
	t.Parallel()

	store, ctx := newTestStore(t)
	fs := store.For(1003)

	if err := fs.Mkdir(ctx, "/a"); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	if err := fs.Mkdir(ctx, "/a/b"); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	if err := fs.Rename(ctx, "/a", "/a/b/a"); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("Rename error = %v, want invalid path", err)
	}
}

func TestFSDeepPath(t *testing.T) {
	t.Parallel()

	store, ctx := newTestStore(t)
	fs := store.For(1004)

	path := ""
	for i := range 64 {
		path += fmt.Sprintf("/d%d", i)

		if err := fs.Mkdir(ctx, path); err != nil {
			t.Fatalf("Mkdir(%q): %v", path, err)
		}
	}

	if _, err := fs.Stat(ctx, path); err != nil {
		t.Fatalf("Stat(%q): %v", path, err)
	}
}

func TestFSListPagination(t *testing.T) {
	t.Parallel()

	store, ctx := newTestStore(t)
	fs := store.For(1005)

	if err := fs.Mkdir(ctx, "/a"); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	for i := range 300 {
		name := fmt.Sprintf("/a/f%03d", i)
		if err := fs.Put(ctx, name, bytes.NewBufferString("x")); err != nil {
			t.Fatalf("Put(%q): %v", name, err)
		}
	}

	page, err := fs.List(ctx, "/a", nil)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(page.Inodes) != PageSize || page.Cursor == nil {
		t.Fatalf("first page = %d inodes, cursor %v; want %d and a cursor", len(page.Inodes), page.Cursor, PageSize)
	}

	page, err = fs.List(ctx, "/a", page.Cursor)
	if err != nil {
		t.Fatalf("List second page: %v", err)
	}

	if len(page.Inodes) != 300-PageSize || page.Cursor != nil {
		t.Fatalf("second page = %d inodes, cursor %v", len(page.Inodes), page.Cursor)
	}
}

func TestFSListRoot(t *testing.T) {
	t.Parallel()

	store, ctx := newTestStore(t)
	fs := store.For(1006)

	if err := fs.Mkdir(ctx, "/a"); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	page, err := fs.List(ctx, "/", nil)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(page.Inodes) != 1 || page.Inodes[0].Name != "a" {
		t.Fatalf("List = %+v, want one a", page.Inodes)
	}
}

func TestRevisionConflict(t *testing.T) {
	t.Parallel()

	db := newMemDB()
	ctx := context.Background()

	tx, err := db.Begin(ctx, 0)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}

	if _, err := db.Begin(ctx, 0); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("second Begin error = %v, want %v", err, ErrRevisionConflict)
	}

	if _, err := tx.Put(ctx, Inode{
		UID:      1,
		ParentID: RootNodeID,
		Name:     "a",
		Kind:     KindDirectory,
	}); err != nil {
		t.Fatalf("Put: %v", err)
	}

	if err := tx.Commit(ctx, nil); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	next, err := db.Begin(ctx, 1)
	if err != nil {
		t.Fatalf("Begin after commit: %v", err)
	}

	if next.Revision() != 2 {
		t.Fatalf("revision = %d, want 2", next.Revision())
	}
}

func TestFSWriteRetryOnConflict(t *testing.T) {
	t.Parallel()

	db := newMemDB()
	store, ctx := newTestStoreWith(t, db, newMemBlob(), newMemNotifications())
	fs := store.For(1007)

	if err := fs.Mkdir(ctx, "/seed"); err != nil {
		t.Fatalf("seed Mkdir: %v", err)
	}

	gated := &gateDB{
		memDB:    db,
		began:    make(chan struct{}, 1),
		conflict: make(chan struct{}, 16),
		gate:     make(chan struct{}, 1),
	}
	gated.gate <- struct{}{}

	store.db = gated

	errs := make(chan error, 2)

	go func() { errs <- fs.Mkdir(ctx, "/a") }()

	// Wait for the first writer to hold the write transaction.
	<-gated.began

	go func() { errs <- fs.Mkdir(ctx, "/b") }()

	for range 2 {
		if err := <-errs; err != nil {
			t.Fatalf("Mkdir after conflict: %v", err)
		}
	}

	for _, path := range []string{"/a", "/b"} {
		if _, err := fs.Stat(ctx, path); err != nil {
			t.Fatalf("Stat(%q): %v", path, err)
		}
	}
}

type gateDB struct {
	*memDB

	began    chan struct{}
	conflict chan struct{}
	gate     chan struct{}
}

func (d *gateDB) Begin(ctx context.Context, expected uint64) (Tx, error) {
	tx, err := d.memDB.Begin(ctx, expected)
	if err != nil {
		if errors.Is(err, ErrRevisionConflict) {
			select {
			case d.conflict <- struct{}{}:
			default:
			}
		}

		return nil, err
	}

	select {
	case d.began <- struct{}{}:
	default:
	}

	select {
	case <-d.gate:
		return &gatedTx{Tx: tx, conflict: d.conflict}, nil
	default:
		return tx, nil
	}
}

type gatedTx struct {
	Tx

	conflict chan struct{}
}

func (t *gatedTx) Commit(ctx context.Context, payload []byte) error {
	select {
	case <-t.conflict:
	case <-ctx.Done():
		return ctx.Err()
	}

	return t.Tx.Commit(ctx, payload)
}

func TestCacheRebuild(t *testing.T) {
	t.Parallel()

	store, ctx := newTestStore(t)
	fs := store.For(1007)

	if err := fs.Mkdir(ctx, "/a"); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	if err := fs.Put(ctx, "/a/f", bytes.NewBufferString("hello")); err != nil {
		t.Fatalf("Put: %v", err)
	}

	revision := store.Revision()

	if err := store.bootstrap(ctx); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	inode, err := fs.Stat(ctx, "/a/f")
	if err != nil {
		t.Fatalf("Stat after rebuild: %v", err)
	}

	if inode.Name != "f" || inode.Size != 5 {
		t.Fatalf("Stat after rebuild = %#v, want file f with size 5", inode)
	}

	if got := store.Revision(); got != revision {
		t.Fatalf("revision after rebuild = %d, want %d", got, revision)
	}
}

func TestSnapshot(t *testing.T) {
	t.Parallel()

	store, ctx := newTestStore(t)
	fs := store.For(1006)

	if err := fs.Mkdir(ctx, "/a"); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	if err := fs.Put(ctx, "/a/f", bytes.NewBufferString("hello")); err != nil {
		t.Fatalf("Put: %v", err)
	}

	var (
		revision uint64
		inodes   []Inode
	)

	err := store.Snapshot(ctx, func(pageRevision uint64, page []Inode) error {
		revision = pageRevision

		inodes = append(inodes, page...)

		return nil
	})
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	if revision == 0 {
		t.Fatal("snapshot revision = 0, want a positive value")
	}

	if len(inodes) != 3 {
		t.Fatalf("snapshot inodes = %d, want 3", len(inodes))
	}
}

type readyService chan struct{}

func (s readyService) Run(step run.Step) error {
	select {
	case s <- struct{}{}:
	default:
	}

	step.Notify()

	<-step.Context().Done()

	return nil
}

func newTestStore(t *testing.T) (*Store, context.Context) {
	t.Helper()

	return newTestStoreWith(t, newMemDB(), newMemBlob(), newMemNotifications())
}

func newTestStoreWith(
	t *testing.T,
	db *memDB,
	blob *memBlob,
	notifications *memNotifications,
) (*Store, context.Context) {
	t.Helper()

	logger := slog.New(slog.DiscardHandler)

	store, err := NewStore(Config{
		Cache: CacheConfig{
			Path:   filepath.Join(t.TempDir(), "cache"),
			Memory: 1 << 20,
			Disk:   32 << 20,
		},
		RetryDelay: time.Millisecond,
		MaxRetries: 5,
	}, db, blob, notifications, logger)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	ready := make(readyService, 1)
	errs := make(chan error, 1)

	runner := run.NewRun(run.RunConfig{Timeout: 10 * time.Second}, logger)
	runner.Add("store", store, nil)
	runner.Add("ready", ready, nil)

	go func() {
		errs <- runner.Run(ctx)
	}()

	select {
	case <-ready:
	case err := <-errs:
		cancel()
		t.Fatalf("Run: %v", err)
	case <-time.After(10 * time.Second):
		cancel()
		t.Fatal("store did not become ready")
	}

	t.Cleanup(func() {
		cancel()

		select {
		case err := <-errs:
			if err != nil {
				t.Errorf("Run returned an error: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("store did not stop")
		}
	})

	return store, ctx
}
