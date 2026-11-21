package remotefiles

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/teapotovh/teapot/lib/pagedbtree"
	"github.com/teapotovh/teapot/lib/run"
	filespb "github.com/teapotovh/teapot/proto/files"
)

const (
	cacheDirMode  = os.FileMode(0o700)
	maxDirtyPages = 64
)

// CacheConfig configures the local cache stores.
type CacheConfig struct {
	// Inodes configures the inode store.
	Inodes TreeConfig
	// Dirents configures the directory entry index store.
	Dirents TreeConfig
}

// TreeConfig configures one paged B-tree store.
type TreeConfig struct {
	// Path is the file used for the store.
	Path string
	// Memory is the page cache budget in bytes. Zero disables page caching.
	Memory int64
	// Disk is the maximum file size in bytes. Zero means no limit.
	Disk int64
}

// cache is the local in-memory and disk-backed view of the file tree.
type cache struct {
	inodes  *pagedbtree.Bytes
	dirents *pagedbtree.Bytes

	inodesReady  chan struct{}
	direntsReady chan struct{}
}

func newCache(config CacheConfig) (*cache, error) {
	inodes, err := openTree(config.Inodes)
	if err != nil {
		return nil, fmt.Errorf("could not open inode cache: %w", err)
	}

	dirents, err := openTree(config.Dirents)
	if err != nil {
		return nil, fmt.Errorf("could not open dirent cache: %w", err)
	}

	return &cache{
		inodes:       inodes,
		dirents:      dirents,
		inodesReady:  make(chan struct{}, 1),
		direntsReady: make(chan struct{}, 1),
	}, nil
}

func (c *cache) wait(ctx context.Context) bool {
	for _, ready := range []chan struct{}{c.inodesReady, c.direntsReady} {
		select {
		case <-ready:
		case <-ctx.Done():
			return false
		}
	}

	return true
}

type treeRunner struct {
	tree  *pagedbtree.Bytes
	ready chan<- struct{}
}

// Run implements run.Runnable.
func (r *treeRunner) Run(ctx context.Context, notify run.Notify) error {
	return r.tree.Run(ctx, treeNotify{ready: r.ready, next: notify})
}

type treeNotify struct {
	ready chan<- struct{}
	next  run.Notify
}

// Notify implements run.Notify.
func (n treeNotify) Notify() {
	n.next.Notify()

	select {
	case n.ready <- struct{}{}:
	default:
	}
}

func openTree(config TreeConfig) (*pagedbtree.Bytes, error) {
	if err := os.MkdirAll(filepath.Dir(config.Path), cacheDirMode); err != nil {
		return nil, fmt.Errorf("could not create cache directory: %w", err)
	}

	if err := os.Remove(config.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("could not remove cache file %q: %w", config.Path, err)
	}

	return pagedbtree.Open(osFileSystem{}, pagedbtree.Config{
		Path:       config.Path,
		Memory:     config.Memory,
		Disk:       config.Disk,
		DirtyPages: maxDirtyPages,
	})
}

func (c *cache) inode(uid int64, nodeID uint64) (*filespb.Inode, bool, error) {
	data, ok, err := c.inodes.Load(inodeKey(uid, nodeID))
	if err != nil || !ok {
		return nil, ok, err
	}

	inode := new(filespb.Inode)
	if err := inode.UnmarshalVT(data); err != nil {
		return nil, false, fmt.Errorf("could not decode inode: %w", err)
	}

	return inode, true, nil
}

func (c *cache) put(inode *filespb.Inode) error {
	identifier := inode.GetIdentifier()
	if identifier == nil {
		return ErrInvalidInode
	}

	old, found, err := c.inode(identifier.GetUid(), identifier.GetNodeId())
	if err != nil {
		return err
	}

	data, err := inode.MarshalVT()
	if err != nil {
		return fmt.Errorf("could not encode inode: %w", err)
	}

	if err := c.inodes.Store(inodeKey(identifier.GetUid(), identifier.GetNodeId()), data); err != nil {
		return fmt.Errorf("could not store inode: %w", err)
	}

	if found && (old.GetParentId() != inode.GetParentId() || old.GetName() != inode.GetName()) {
		if err := c.dirents.Delete(direntKey(identifier.GetUid(), old.GetParentId(), old.GetName())); err != nil {
			return fmt.Errorf("could not remove old dirent: %w", err)
		}
	}

	// The root is its own parent, so it is not a dirent of any inode.
	if identifier.GetNodeId() != inode.GetParentId() {
		if err := c.dirents.Store(
			direntKey(identifier.GetUid(), inode.GetParentId(), inode.GetName()),
			encodeNodeID(identifier.GetNodeId()),
		); err != nil {
			return fmt.Errorf("could not store dirent: %w", err)
		}
	}

	return nil
}

func (c *cache) delete(uid int64, nodeID uint64) (bool, error) {
	inode, ok, err := c.inode(uid, nodeID)
	if err != nil || !ok {
		return ok, err
	}

	if err := c.inodes.Delete(inodeKey(uid, nodeID)); err != nil {
		return false, fmt.Errorf("could not delete inode: %w", err)
	}

	if inode.GetIdentifier().GetNodeId() != inode.GetParentId() {
		if err := c.dirents.Delete(direntKey(uid, inode.GetParentId(), inode.GetName())); err != nil {
			return false, fmt.Errorf("could not delete dirent: %w", err)
		}
	}

	return true, nil
}

func (c *cache) dirent(uid int64, parentID uint64, name string) (uint64, bool, error) {
	data, ok, err := c.dirents.Load(direntKey(uid, parentID, name))
	if err != nil || !ok {
		return 0, ok, err
	}

	nodeID, err := decodeNodeID(data)
	if err != nil {
		return 0, false, err
	}

	return nodeID, true, nil
}

func (c *cache) clear() error {
	if err := clearStore(c.inodes); err != nil {
		return fmt.Errorf("could not clear inode cache: %w", err)
	}

	if err := clearStore(c.dirents); err != nil {
		return fmt.Errorf("could not clear dirent cache: %w", err)
	}

	return nil
}

func clearStore(store *pagedbtree.Bytes) error {
	keys := []string{}
	for key := range store.All() {
		keys = append(keys, key)
	}

	for _, key := range keys {
		if err := store.Delete(key); err != nil {
			return err
		}
	}

	return nil
}

func inodeKey(uid int64, nodeID uint64) string {
	key := make([]byte, 16)
	binary.BigEndian.PutUint64(key[0:], uint64(uid)) //nolint:gosec // user ids are non-negative.
	binary.BigEndian.PutUint64(key[8:], nodeID)

	return string(key)
}

func direntPrefix(uid int64, parentID uint64) string {
	key := make([]byte, 16)
	binary.BigEndian.PutUint64(key[0:], uint64(uid)) //nolint:gosec // user ids are non-negative.
	binary.BigEndian.PutUint64(key[8:], parentID)

	return string(key)
}

func direntKey(uid int64, parentID uint64, name string) string {
	return direntPrefix(uid, parentID) + name
}

func encodeNodeID(nodeID uint64) []byte {
	data := make([]byte, 8)
	binary.BigEndian.PutUint64(data, nodeID)

	return data
}

func decodeNodeID(data []byte) (uint64, error) {
	if len(data) != 8 {
		return 0, ErrInvalidInode
	}

	return binary.BigEndian.Uint64(data), nil
}

type osFileSystem struct{}

// OpenFile implements pagedbtree.FS.
func (osFileSystem) OpenFile(name string, flag int, perm os.FileMode) (pagedbtree.File, error) {
	return os.OpenFile(name, flag, perm) //nolint:gosec // the cache path comes from configuration.
}

var _ pagedbtree.FS = osFileSystem{}
