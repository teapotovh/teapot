package store

import (
	"bytes"
	"context"
	"crypto/md5" //nolint:gosec
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/teapotovh/teapot/lib/files"
	"github.com/teapotovh/teapot/lib/pgnotify"
)

func TestFSWriteRead(t *testing.T) {
	t.Parallel()

	store, ctx := newTestStore(t)
	fs := store.For(1000)

	if err := fs.Mkdir(ctx, "/a"); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	if err := fs.Put(ctx, "/a/f.txt", bytes.NewBufferString("hello"), 0o644); err != nil {
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

	if _, err := fs.Stat(ctx, "/a/f.txt"); !errors.Is(err, files.ErrNotFound) {
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
		if err := fs.Put(ctx, path, bytes.NewBufferString("x"), 0o644); !errors.Is(err, files.ErrInvalidPath) {
			t.Fatalf("Put(%q) error = %v, want invalid path", path, err)
		}
	}

	for _, path := range []string{"/a/b\x01c"} {
		if err := fs.Put(ctx, path, bytes.NewBufferString("x"), 0o644); !errors.Is(err, files.ErrInvalidName) {
			t.Fatalf("Put(%q) error = %v, want invalid name", path, err)
		}
	}

	for _, path := range []string{"/a/..", "/a/../b"} {
		if err := fs.Put(ctx, path, bytes.NewBufferString("x"), 0o644); !errors.Is(err, files.ErrInvalidPath) {
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

	if err := fs.Put(ctx, "/a/f", bytes.NewBufferString("x"), 0o644); err != nil {
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

	if err := fs.Rename(ctx, "/a", "/a/b/a"); !errors.Is(err, files.ErrInvalidPath) {
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
		if err := fs.Put(ctx, name, bytes.NewBufferString("x"), 0o644); err != nil {
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

func TestSnapshot(t *testing.T) {
	t.Parallel()

	store, ctx := newTestStore(t)
	fs := store.For(1006)

	if err := fs.Mkdir(ctx, "/a"); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	if err := fs.Put(ctx, "/a/f", bytes.NewBufferString("hello"), 0o644); err != nil {
		t.Fatalf("Put: %v", err)
	}

	var (
		revision uint64
		inodes   []Inode
	)

	err := store.Snapshot(ctx, 0, func(pageRevision uint64, page []Inode) error {
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

type memState struct {
	revision uint64
	inodes   map[int64]map[uint64]Inode
}

type memDB struct {
	mu    sync.Mutex
	state *memState
}

func newMemDB() *memDB {
	return &memDB{state: &memState{inodes: map[int64]map[uint64]Inode{}}}
}

func (d *memDB) Begin(context.Context) (Tx, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	return &memTx{db: d, state: d.copyLocked()}, nil
}

func (d *memDB) BeginSnapshot(ctx context.Context) (Tx, error) {
	return d.Begin(ctx)
}

func (d *memDB) Ping(context.Context) error { return nil }
func (d *memDB) Close()                     {}

func (d *memDB) copyLocked() *memState {
	state := &memState{revision: d.state.revision, inodes: map[int64]map[uint64]Inode{}}
	for uid, nodes := range d.state.inodes {
		copied := map[uint64]Inode{}
		maps.Copy(copied, nodes)

		state.inodes[uid] = copied
	}

	return state
}

type memTx struct {
	db    *memDB
	state *memState
}

func (t *memTx) BumpRevision(context.Context) (uint64, error) {
	t.state.revision++

	return t.state.revision, nil
}

func (t *memTx) CurrentRevision(context.Context) (uint64, error) {
	return t.state.revision, nil
}

func (t *memTx) EnsureRoot(_ context.Context, uid int64, revision uint64) error {
	nodes := t.state.inodes[uid]
	if nodes == nil {
		nodes = map[uint64]Inode{}
		t.state.inodes[uid] = nodes
	}

	if _, ok := nodes[RootNodeID]; ok {
		return nil
	}

	nodes[RootNodeID] = Inode{
		UID:      uid,
		NodeID:   RootNodeID,
		ParentID: RootNodeID,
		Kind:     files.KindDirectory,
		Revision: revision,
	}

	return nil
}

func (t *memTx) NextNodeID(_ context.Context, uid int64) (uint64, error) {
	var maxID uint64
	for id := range t.state.inodes[uid] {
		if id > maxID {
			maxID = id
		}
	}

	return maxID + 1, nil
}

func (t *memTx) Inode(_ context.Context, uid int64, nodeID uint64) (Inode, bool, error) {
	inode, ok := t.state.inodes[uid][nodeID]
	if !ok || inode.Deleted {
		return Inode{}, false, nil
	}

	return inode, true, nil
}

func (t *memTx) Child(_ context.Context, uid int64, parentID uint64, name string) (Inode, bool, error) {
	var result Inode

	found := false

	for _, inode := range t.state.inodes[uid] {
		if inode.Deleted || inode.ParentID != parentID || inode.Name != name {
			continue
		}

		if !found || inode.NodeID > result.NodeID {
			result = inode
			found = true
		}
	}

	return result, found, nil
}

func (t *memTx) Children(_ context.Context, uid int64, parentID uint64) ([]Inode, error) {
	children := make([]Inode, 0)

	for _, inode := range t.state.inodes[uid] {
		if !inode.Deleted && inode.ParentID == parentID {
			children = append(children, inode)
		}
	}

	slices.SortFunc(children, func(a, b Inode) int { return strings.Compare(a.Name, b.Name) })

	return children, nil
}

func (t *memTx) Upsert(_ context.Context, inode Inode) error {
	nodes := t.state.inodes[inode.UID]
	if nodes == nil {
		nodes = map[uint64]Inode{}
		t.state.inodes[inode.UID] = nodes
	}

	nodes[inode.NodeID] = inode

	return nil
}

func (t *memTx) ChangedSince(_ context.Context, revision uint64) ([]Inode, error) {
	inodes := make([]Inode, 0)
	for _, nodes := range t.state.inodes {
		for _, inode := range nodes {
			if inode.Revision > revision {
				inodes = append(inodes, inode)
			}
		}
	}

	slices.SortFunc(inodes, func(a, b Inode) int {
		switch {
		case a.Revision < b.Revision:
			return -1
		case a.Revision > b.Revision:
			return 1
		default:
			return 0
		}
	})

	return inodes, nil
}

func (t *memTx) SnapshotPage(
	_ context.Context,
	afterRevision uint64,
	cursor Cursor,
	limit int,
) ([]Inode, Cursor, bool, error) {
	inodes := make([]Inode, 0)
	for _, nodes := range t.state.inodes {
		for _, inode := range nodes {
			if inode.Revision <= afterRevision {
				continue
			}

			if afterRevision == 0 && inode.Deleted {
				continue
			}

			inodes = append(inodes, inode)
		}
	}

	slices.SortFunc(inodes, func(a, b Inode) int {
		if a.ParentID != b.ParentID {
			if a.ParentID < b.ParentID {
				return -1
			}

			return 1
		}

		return strings.Compare(a.Name, b.Name)
	})

	start := 0

	for i, inode := range inodes {
		if inode.ParentID > cursor.ParentID ||
			(inode.ParentID == cursor.ParentID && inode.Name > cursor.Name) {
			start = i
			break
		}

		start = i + 1
	}

	inodes = inodes[start:]

	more := len(inodes) > limit
	if more {
		inodes = inodes[:limit]
	}

	next := cursor

	if len(inodes) > 0 {
		last := inodes[len(inodes)-1]
		next = Cursor{ParentID: last.ParentID, Name: last.Name}
	}

	return inodes, next, more, nil
}

func (t *memTx) OldestTombstone(context.Context) (uint64, bool, error) {
	var oldest uint64

	found := false

	for _, nodes := range t.state.inodes {
		for _, inode := range nodes {
			if inode.Deleted && (!found || inode.Revision < oldest) {
				oldest = inode.Revision
				found = true
			}
		}
	}

	return oldest, found, nil
}

func (t *memTx) Notify(context.Context, []byte) error { return nil }

func (t *memTx) Commit(context.Context) error {
	t.db.mu.Lock()
	defer t.db.mu.Unlock()

	t.db.state = t.state

	return nil
}

func (t *memTx) Rollback(context.Context) error { return nil }

type memBlob struct {
	mu    sync.Mutex
	blobs map[string][]byte
}

func newMemBlob() *memBlob {
	return &memBlob{blobs: map[string][]byte{}}
}

func (b *memBlob) Put(_ context.Context, key string, r io.Reader) (files.Hash, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}

	b.mu.Lock()
	b.blobs[key] = data
	b.mu.Unlock()

	sum := md5.Sum(data) //nolint:gosec

	return files.Hash(hex.EncodeToString(sum[:])), nil
}

func (b *memBlob) Get(_ context.Context, key string) (io.ReadSeekCloser, error) {
	b.mu.Lock()
	data, ok := b.blobs[key]
	b.mu.Unlock()

	if !ok {
		return nil, os.ErrNotExist
	}

	return &readSeekCloser{Reader: bytes.NewReader(data)}, nil
}

type readSeekCloser struct {
	*bytes.Reader
}

func (r *readSeekCloser) Close() error { return nil }

type memNotifications struct {
	ch chan []byte
}

func newMemNotifications() *memNotifications {
	return &memNotifications{ch: make(chan []byte, 16)}
}

func (n *memNotifications) Listen(context.Context, string) error { return nil }

func (n *memNotifications) Next(ctx context.Context) (string, []byte, error) {
	select {
	case <-ctx.Done():
		return "", nil, ctx.Err()
	case payload := <-n.ch:
		return "files_test", payload, nil
	}
}

func (n *memNotifications) Notify(context.Context, pgnotify.Executor, string, []byte) error {
	return nil
}

func (n *memNotifications) Close() error {
	return nil
}
