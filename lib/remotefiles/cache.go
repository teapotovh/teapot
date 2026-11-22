package remotefiles

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"

	"github.com/teapotovh/teapot/lib/pagedbtree"
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

type inodeValue struct {
	key   string
	inode *filespb.Inode
}

// Key implements pagedbtree.Keyed.
func (v inodeValue) Key() string {
	return v.key
}

type inodeCodec struct{}

// Encode implements pagedbtree.Codec.
func (inodeCodec) Encode(value inodeValue) ([]byte, error) {
	return value.inode.MarshalVT()
}

// Decode implements pagedbtree.Codec.
func (inodeCodec) Decode(data []byte) (inodeValue, error) {
	inode := new(filespb.Inode)
	if err := inode.UnmarshalVT(data); err != nil {
		return inodeValue{}, fmt.Errorf("could not decode inode: %w", err)
	}

	return inodeValue{inode: inode}, nil
}

type direntValue struct {
	key    string
	nodeID uint64
}

// Key implements pagedbtree.Keyed.
func (v direntValue) Key() string {
	return v.key
}

type direntCodec struct{}

// Encode implements pagedbtree.Codec.
func (direntCodec) Encode(value direntValue) ([]byte, error) {
	return encodeNodeID(value.nodeID), nil
}

// Decode implements pagedbtree.Codec.
func (direntCodec) Decode(data []byte) (direntValue, error) {
	nodeID, err := decodeNodeID(data)
	if err != nil {
		return direntValue{}, err
	}

	return direntValue{nodeID: nodeID}, nil
}

// cache is the local in-memory and disk-backed view of the file tree.
type cache struct {
	inodes  *pagedbtree.Tree[inodeValue]
	dirents *pagedbtree.Tree[direntValue]
}

func newCache(config CacheConfig) (*cache, error) {
	inodes, err := openTree(config.Inodes, inodeCodec{})
	if err != nil {
		return nil, fmt.Errorf("could not open inode cache: %w", err)
	}

	dirents, err := openTree(config.Dirents, direntCodec{})
	if err != nil {
		return nil, fmt.Errorf("could not open dirent cache: %w", err)
	}

	return &cache{
		inodes:  inodes,
		dirents: dirents,
	}, nil
}

func openTree[T pagedbtree.Keyed](
	config TreeConfig,
	codec pagedbtree.Codec[T],
) (*pagedbtree.Tree[T], error) {
	if err := os.MkdirAll(filepath.Dir(config.Path), cacheDirMode); err != nil {
		return nil, fmt.Errorf("could not create cache directory: %w", err)
	}

	return pagedbtree.OpenTree(osFileSystem{}, pagedbtree.Config{
		Path:       config.Path,
		Memory:     config.Memory,
		Disk:       config.Disk,
		DirtyPages: maxDirtyPages,
	}, codec)
}

func (c *cache) inode(uid int64, nodeID uint64) (*filespb.Inode, bool, error) {
	value, ok, err := c.inodes.Load(inodeKey(uid, nodeID))
	if err != nil {
		return nil, false, fmt.Errorf("could not load inode %d: %w", nodeID, err)
	}

	if !ok {
		return nil, false, nil
	}

	return value.inode, true, nil
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

	key := inodeKey(identifier.GetUid(), identifier.GetNodeId())
	if err := c.inodes.Store(inodeValue{key: key, inode: inode}); err != nil {
		return fmt.Errorf("could not store inode: %w", err)
	}

	if found && (old.GetParentId() != inode.GetParentId() || old.GetName() != inode.GetName()) {
		oldKey := direntKey(identifier.GetUid(), old.GetParentId(), old.GetName())
		if err := c.dirents.Delete(oldKey); err != nil {
			return fmt.Errorf("could not remove old dirent: %w", err)
		}
	}

	// The root is its own parent, so it is not a dirent of any inode.
	if identifier.GetNodeId() != inode.GetParentId() {
		key := direntKey(identifier.GetUid(), inode.GetParentId(), inode.GetName())
		if err := c.dirents.Store(direntValue{key: key, nodeID: identifier.GetNodeId()}); err != nil {
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
		key := direntKey(uid, inode.GetParentId(), inode.GetName())
		if err := c.dirents.Delete(key); err != nil {
			return false, fmt.Errorf("could not delete dirent: %w", err)
		}
	}

	return true, nil
}

func (c *cache) dirent(uid int64, parentID uint64, name string) (uint64, bool, error) {
	value, ok, err := c.dirents.Load(direntKey(uid, parentID, name))
	if err != nil {
		return 0, false, fmt.Errorf("could not load directory entry %q: %w", name, err)
	}

	if !ok {
		return 0, false, nil
	}

	return value.nodeID, true, nil
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

func clearStore[T pagedbtree.Keyed](store *pagedbtree.Tree[T]) error {
	keys := []string{}
	for key := range store.All() {
		keys = append(keys, key)
	}

	for _, key := range keys {
		if err := store.Delete(key); err != nil {
			return fmt.Errorf("could not delete cache key: %w", err)
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

// Ensure osFileSystem implements pagedbtree.FS.
var _ pagedbtree.FS = osFileSystem{}
