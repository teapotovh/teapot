package store

import (
	flag "github.com/spf13/pflag"
)

// CacheFlagSet returns the flags for an inode cache config.
func CacheFlagSet() (*flag.FlagSet, func() CacheConfig) {
	fs := flag.NewFlagSet("files/store/cache", flag.ExitOnError)

	path := fs.String("files-store-cache-path", "", "file prefix for the inode cache")
	memory := fs.Int64("files-store-cache-memory", 128<<20, "inode cache memory budget in bytes")
	disk := fs.Int64("files-store-cache-disk", 8<<30, "inode cache maximum file size in bytes")

	return fs, func() CacheConfig {
		return CacheConfig{
			Path:   *path,
			Memory: *memory,
			Disk:   *disk,
		}
	}
}
