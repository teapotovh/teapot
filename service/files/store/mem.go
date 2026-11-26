package store

import (
	"bytes"
	"context"
	"crypto/md5" //nolint:gosec
	"encoding/hex"
	"io"
	"log/slog"
	"maps"
	"os"
	"sync"
	"time"

	"github.com/teapotovh/teapot/lib/pagedbtree"
	"github.com/teapotovh/teapot/lib/pgnotify"
	"github.com/teapotovh/teapot/lib/s3cache"
)

const (
	memCachePath   = "memory"
	memCacheMemory = 1 << 20
)

// NewMem creates an in-memory store.
func NewMem(logger *slog.Logger) (*Store, error) {
	return newStore(
		Config{
			Cache:      CacheConfig{Path: memCachePath, Memory: memCacheMemory},
			RetryDelay: time.Second,
			MaxRetries: 5,
		},
		newMemDB(),
		newMemBlob(),
		newMemNotifications(),
		newMemFS(),
		logger,
	)
}

type memFS struct {
	mu    sync.Mutex
	files map[string]*memFile
}

func newMemFS() *memFS {
	return &memFS{files: map[string]*memFile{}}
}

// OpenFile implements pagedbtree.FS.
func (f *memFS) OpenFile(name string, _ int, _ os.FileMode) (pagedbtree.File, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	file, ok := f.files[name]
	if !ok {
		file = &memFile{}
		f.files[name] = file
	}

	return file, nil
}

type memFile struct {
	mu   sync.Mutex
	data []byte
}

// ReadAt implements pagedbtree.File.
func (f *memFile) ReadAt(p []byte, offset int64) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if offset < 0 {
		return 0, os.ErrInvalid
	}

	if offset >= int64(len(f.data)) {
		return 0, io.EOF
	}

	n := copy(p, f.data[offset:])
	if n < len(p) {
		return n, io.EOF
	}

	return n, nil
}

// WriteAt implements pagedbtree.File.
func (f *memFile) WriteAt(p []byte, offset int64) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	end := offset + int64(len(p))
	if end > int64(len(f.data)) {
		data := make([]byte, end)
		copy(data, f.data)
		f.data = data
	}

	copy(f.data[offset:], p)

	return len(p), nil
}

// Truncate implements pagedbtree.File.
func (f *memFile) Truncate(size int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if size <= int64(len(f.data)) {
		f.data = f.data[:size]

		return nil
	}

	f.data = append(f.data, make([]byte, size-int64(len(f.data)))...)

	return nil
}

// Stat implements pagedbtree.File.
func (f *memFile) Stat() (os.FileInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return memFileInfo{size: int64(len(f.data))}, nil
}

// Close implements pagedbtree.File.
func (f *memFile) Close() error {
	return nil
}

type memFileInfo struct {
	size int64
}

func (i memFileInfo) Name() string       { return memCachePath }
func (i memFileInfo) Size() int64        { return i.size }
func (i memFileInfo) Mode() os.FileMode  { return 0o600 }
func (i memFileInfo) ModTime() time.Time { return time.Time{} }
func (i memFileInfo) IsDir() bool        { return false }
func (i memFileInfo) Sys() any           { return nil }

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

// Begin implements DB.
func (d *memDB) Begin(_ context.Context, expected uint64) (Tx, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.state.revision != expected {
		return nil, ErrRevisionConflict
	}

	d.state.revision++
	state := d.copyLocked()

	return &memTx{db: d, state: state, revision: state.revision}, nil
}

// Load implements DB.
func (d *memDB) Load(_ context.Context, fn func(uint64, []Inode) error) error {
	d.mu.Lock()
	state := d.copyLocked()
	d.mu.Unlock()

	inodes := make([]Inode, 0)

	for _, nodes := range state.inodes {
		for _, inode := range nodes {
			if !inode.Deleted {
				inodes = append(inodes, inode)
			}
		}
	}

	return fn(state.revision, inodes)
}

func (d *memDB) Close() {}

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
	db       *memDB
	state    *memState
	revision uint64
}

// Revision implements Tx.
func (t *memTx) Revision() uint64 {
	return t.revision
}

// Put implements Tx.
func (t *memTx) Put(_ context.Context, inode Inode) (uint64, error) {
	nodes := t.state.inodes[inode.UID]
	if nodes == nil {
		nodes = map[uint64]Inode{}
		t.state.inodes[inode.UID] = nodes
	}

	if inode.NodeID == 0 {
		var maxID uint64
		for id := range nodes {
			if id > maxID {
				maxID = id
			}
		}

		inode.NodeID = maxID + 1
	}

	inode.Revision = t.revision
	nodes[inode.NodeID] = inode

	return inode.NodeID, nil
}

// Delete implements Tx.
func (t *memTx) Delete(_ context.Context, uid int64, nodeID uint64) error {
	delete(t.state.inodes[uid], nodeID)

	return nil
}

// Commit implements Tx.
func (t *memTx) Commit(context.Context, []byte) error {
	t.db.mu.Lock()
	defer t.db.mu.Unlock()

	t.db.state = t.state

	return nil
}

// Rollback implements Tx.
func (t *memTx) Rollback(context.Context) error { return nil }

type memBlob struct {
	mu    sync.Mutex
	blobs map[string][]byte
}

func newMemBlob() *memBlob {
	return &memBlob{blobs: map[string][]byte{}}
}

func (b *memBlob) Put(_ context.Context, key string, r io.Reader) (s3cache.Hash, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}

	b.mu.Lock()
	b.blobs[key] = data
	b.mu.Unlock()

	sum := md5.Sum(data) //nolint:gosec

	return s3cache.Hash(hex.EncodeToString(sum[:])), nil
}

func (b *memBlob) Get(
	_ context.Context,
	key string,
	_ s3cache.Hash,
) (io.ReadSeekCloser, s3cache.Hash, error) {
	b.mu.Lock()
	data, ok := b.blobs[key]
	b.mu.Unlock()

	if !ok {
		return nil, s3cache.ZeroHash, os.ErrNotExist
	}

	sum := md5.Sum(data) //nolint:gosec

	return &readSeekCloser{Reader: bytes.NewReader(data)}, s3cache.Hash(hex.EncodeToString(sum[:])), nil
}

func (b *memBlob) Remove(_ context.Context, key string) error {
	b.mu.Lock()
	delete(b.blobs, key)
	b.mu.Unlock()

	return nil
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

// Ensure *memFS implements pagedbtree.FS.
var _ pagedbtree.FS = (*memFS)(nil)

// Ensure *memFile implements pagedbtree.File.
var _ pagedbtree.File = (*memFile)(nil)

// Ensure *memDB implements DB.
var _ DB = (*memDB)(nil)

// Ensure *memTx implements Tx.
var _ Tx = (*memTx)(nil)

// Ensure *memBlob implements s3cache.Blob.
var _ s3cache.Blob = (*memBlob)(nil)

// Ensure *memNotifications implements pgnotify.Notifications.
var _ pgnotify.Notifications = (*memNotifications)(nil)
