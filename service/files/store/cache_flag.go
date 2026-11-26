package store

import (
	flag "github.com/spf13/pflag"
)

const (
	defaultCacheMemory = 128 << 20
	defaultCacheDisk   = 8 << 30
)

// CacheFlagSet returns the flags for an inode cache config.
func CacheFlagSet() (*flag.FlagSet, func() CacheConfig) {
	fs := flag.NewFlagSet("files/store/cache", flag.ExitOnError)

	path := fs.String("files-store-cache-path", "", "directory for the inode cache files")
	inodesMemory := fs.Int64(
		"files-store-cache-inodes-memory",
		defaultCacheMemory,
		"inode page cache memory budget in bytes",
	)
	childrenMemory := fs.Int64(
		"files-store-cache-children-memory",
		defaultCacheMemory,
		"children page cache memory budget in bytes",
	)
	disk := fs.Int64("files-store-cache-disk", defaultCacheDisk, "inode cache maximum file size in bytes")

	return fs, func() CacheConfig {
		return CacheConfig{
			Path:           *path,
			InodesMemory:   *inodesMemory,
			ChildrenMemory: *childrenMemory,
			Disk:           *disk,
		}
	}
}
