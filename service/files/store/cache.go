package store

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/teapotovh/teapot/lib/files"
	"github.com/teapotovh/teapot/lib/pagedbtree"
)

// ErrCorruptData reports cached data that cannot be decoded.
var ErrCorruptData = errors.New("corrupt data")

// CacheConfig is the configuration for the inode cache.
type CacheConfig struct {
	// Path is the file prefix used by the cache.
	Path string
	// Memory is the page cache budget in bytes.
	Memory int64
	// Disk is the maximum cache file size in bytes.
	Disk int64
}

type cache struct {
	nodes    *pagedbtree.Bytes
	children *pagedbtree.Bytes

	mu sync.RWMutex
}

func openCache(fs pagedbtree.FS, cfg pagedbtree.Config) (*cache, error) {
	nodeCfg := cfg
	nodeCfg.Path = cfg.Path + ".inodes"

	childCfg := cfg
	childCfg.Path = cfg.Path + ".children"

	nodes, err := pagedbtree.Open(fs, nodeCfg)
	if err != nil {
		return nil, fmt.Errorf("error while opening the inode cache: %w", err)
	}

	children, err := pagedbtree.Open(fs, childCfg)
	if err != nil {
		return nil, fmt.Errorf("error while opening the children cache: %w", err)
	}

	return &cache{nodes: nodes, children: children}, nil
}

func (c *cache) loadLocked(uid int64, nodeID uint64) (Inode, bool, error) {
	data, ok, err := c.nodes.Load(nodeKeyString(uid, nodeID))
	if err != nil || !ok {
		return Inode{}, ok, err
	}

	inode, err := decodeInode(data)
	if err != nil {
		return Inode{}, false, err
	}

	return inode, true, nil
}

func (c *cache) childLocked(uid int64, parentID uint64, name string) (uint64, bool, error) {
	data, ok, err := c.children.Load(childKeyString(uid, parentID, name))
	if err != nil || !ok {
		return 0, ok, err
	}

	if len(data) != 8 {
		return 0, false, ErrCorruptData
	}

	return binary.BigEndian.Uint64(data), true, nil
}

func (c *cache) resolve(uid int64, path string) (Inode, error) {
	clean, err := files.CleanPath(path)
	if err != nil {
		return Inode{}, err
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	if clean == "/" {
		return c.rootLocked(uid)
	}

	current, err := c.rootLocked(uid)
	if err != nil {
		return Inode{}, err
	}

	for name := range strings.SplitSeq(strings.TrimPrefix(clean, "/"), "/") {
		nodeID, ok, err := c.childLocked(uid, current.NodeID, name)
		if err != nil {
			return Inode{}, err
		}

		if !ok {
			return Inode{}, files.ErrNotFound
		}

		inode, ok, err := c.loadLocked(uid, nodeID)
		if err != nil {
			return Inode{}, err
		}

		if !ok || inode.Deleted {
			return Inode{}, files.ErrNotFound
		}

		current = inode
	}

	return current, nil
}

func (c *cache) rootLocked(uid int64) (Inode, error) {
	inode, ok, err := c.loadLocked(uid, RootNodeID)
	if err != nil {
		return Inode{}, err
	}

	if ok && !inode.Deleted {
		return inode, nil
	}

	return syntheticRoot(uid), nil
}

func (c *cache) childrenPage(
	uid int64,
	parentID uint64,
	cursor *files.Cursor,
	limit int,
) ([]Inode, *files.Cursor, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	prefix := childPrefix(uid, parentID)

	start := prefix
	if cursor != nil {
		start = prefix + string(*cursor) + "\x00"
	}

	entries := make([]Inode, 0, limit+1)

	for key, data := range c.children.Between(start, prefixEnd(prefix)) {
		if len(entries) == limit+1 {
			break
		}

		if len(data) != 8 {
			return nil, nil, ErrCorruptData
		}

		nodeID := binary.BigEndian.Uint64(data)

		inode, ok, err := c.loadLocked(uid, nodeID)
		if err != nil {
			return nil, nil, err
		}

		if !ok || inode.Deleted {
			continue
		}

		inode.Name = strings.TrimPrefix(key, prefix)
		entries = append(entries, inode)
	}

	var next *files.Cursor

	if len(entries) > limit {
		entries = entries[:limit]
		name := entries[limit-1].Name
		next = (*files.Cursor)(&name)
	}

	return entries, next, nil
}

func (c *cache) apply(inode Inode) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	old, ok, err := c.loadLocked(inode.UID, inode.NodeID)
	if err != nil {
		return err
	}

	if ok && old.Revision > inode.Revision {
		return nil
	}

	if ok && (old.ParentID != inode.ParentID || old.Name != inode.Name) {
		if err := c.deleteChildLocked(old.UID, old.ParentID, old.Name, old.NodeID); err != nil {
			return err
		}
	}

	if inode.Deleted {
		if err := c.deleteChildLocked(inode.UID, inode.ParentID, inode.Name, inode.NodeID); err != nil {
			return err
		}

		return c.nodes.Delete(nodeKeyString(inode.UID, inode.NodeID))
	}

	data, err := encodeInode(inode)
	if err != nil {
		return err
	}

	if err := c.nodes.Store(nodeKeyString(inode.UID, inode.NodeID), data); err != nil {
		return err
	}

	return c.children.Store(childKeyString(inode.UID, inode.ParentID, inode.Name), nodeIDBytes(inode.NodeID))
}

func (c *cache) deleteChildLocked(uid int64, parentID uint64, name string, nodeID uint64) error {
	current, ok, err := c.childLocked(uid, parentID, name)
	if err != nil || !ok || current != nodeID {
		return err
	}

	return c.children.Delete(childKeyString(uid, parentID, name))
}

func (c *cache) clear() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	nodeKeys := make([]string, 0)
	for key := range c.nodes.All() {
		nodeKeys = append(nodeKeys, key)
	}

	for _, key := range nodeKeys {
		if err := c.nodes.Delete(key); err != nil {
			return err
		}
	}

	childKeys := make([]string, 0)
	for key := range c.children.All() {
		childKeys = append(childKeys, key)
	}

	for _, key := range childKeys {
		if err := c.children.Delete(key); err != nil {
			return err
		}
	}

	return nil
}

func syntheticRoot(uid int64) Inode {
	return Inode{
		UID:      uid,
		NodeID:   RootNodeID,
		ParentID: RootNodeID,
		Kind:     files.KindDirectory,
	}
}

func nodeKeyString(uid int64, nodeID uint64) string {
	data := make([]byte, 0, 17)
	data = binary.BigEndian.AppendUint64(data, uint64(uid)) //nolint:gosec // UIDs are non-negative.
	data = append(data, 0)
	data = binary.BigEndian.AppendUint64(data, nodeID)

	return string(data)
}

func childPrefix(uid int64, parentID uint64) string {
	data := make([]byte, 0, 18)
	data = binary.BigEndian.AppendUint64(data, uint64(uid)) //nolint:gosec // UIDs are non-negative.
	data = append(data, 0)
	data = binary.BigEndian.AppendUint64(data, parentID)
	data = append(data, 0)

	return string(data)
}

func childKeyString(uid int64, parentID uint64, name string) string {
	return childPrefix(uid, parentID) + name
}

func prefixEnd(prefix string) string {
	if len(prefix) == 0 {
		return prefix
	}

	data := []byte(prefix)
	data[len(data)-1]++

	return string(data)
}

func nodeIDBytes(nodeID uint64) []byte {
	data := make([]byte, 8)
	binary.BigEndian.PutUint64(data, nodeID)

	return data
}
