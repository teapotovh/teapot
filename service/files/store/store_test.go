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

type testNotify chan struct{}

// Notify implements run.Notify.
func (n testNotify) Notify() {
	select {
	case n <- struct{}{}:
	default:
	}
}

func newTestStore(t *testing.T) (*Store, context.Context) {
	t.Helper()

	db := newMemDB()
	blob := newMemBlob()
	notifications := newMemNotifications()

	logger := slog.New(slog.DiscardHandler)

	store, err := NewStore(Config{
		Cache: CacheConfig{
			Path:   filepath.Join(t.TempDir(), "cache"),
			Memory: 1 << 20,
			Disk:   32 << 20,
		},
	}, db, blob, notifications, logger)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	ready := make(testNotify, 1)
	errs := make(chan error, 1)

	go func() {
		errs <- store.Run(ctx, ready)
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
