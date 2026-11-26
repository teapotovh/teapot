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

	"github.com/teapotovh/teapot/lib/fs"
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
		Config{Cache: CacheConfig{Path: memCachePath, Memory: memCacheMemory}},
		newMemDB(),
		newMemBlob(),
		newMemNotifications(),
		fs.NewMemoryFS(),
		logger,
	)
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

// Ensure *memDB implements DB.
var _ DB = (*memDB)(nil)

// Ensure *memTx implements Tx.
var _ Tx = (*memTx)(nil)

// Ensure *memBlob implements s3cache.Blob.
var _ s3cache.Blob = (*memBlob)(nil)

// Ensure *memNotifications implements pgnotify.Notifications.
var _ pgnotify.Notifications = (*memNotifications)(nil)
