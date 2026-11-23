package remotefiles

import (
	flag "github.com/spf13/pflag"
)

// RemoteFilesFlagSet returns the flags for a remote file system config.
func RemoteFilesFlagSet() (*flag.FlagSet, func() Config) {
	fs := flag.NewFlagSet("remotefiles", flag.ExitOnError)

	inodesPath := fs.String(
		"remotefiles-inodes-path",
		"/remotefiles/inodes",
		"the path of the file that stores the inode cache. The remote file system deletes this file when it starts",
	)
	inodesMemory := fs.Int64(
		"remotefiles-inodes-memory",
		16<<20,
		"the maximum memory in bytes used to cache inode pages. Set to 0 to disable the memory page cache",
	)
	inodesDisk := fs.Int64(
		"remotefiles-inodes-disk",
		8<<30,
		"the maximum size in bytes of the inode cache file. Set to 0 to allow the file to grow without a limit",
	)

	direntsPath := fs.String(
		"remotefiles-dirents-path",
		"/remotefiles/dirents",
		"the path of the file that stores the dirent index cache. The remote file system deletes this file when it starts",
	)
	direntsMemory := fs.Int64(
		"remotefiles-dirents-memory",
		16<<20,
		"the maximum memory in bytes used to cache dirent index pages. Set to 0 to disable the memory page cache",
	)
	direntsDisk := fs.Int64(
		"remotefiles-dirents-disk",
		8<<30,
		"the maximum size in bytes of the dirent index cache file. Set to 0 to allow the file to grow without a limit",
	)

	return fs, func() Config {
		return Config{
			Cache: CacheConfig{
				Inodes: TreeConfig{
					Path:   *inodesPath,
					Memory: *inodesMemory,
					Disk:   *inodesDisk,
				},
				Dirents: TreeConfig{
					Path:   *direntsPath,
					Memory: *direntsMemory,
					Disk:   *direntsDisk,
				},
			},
		}
	}
}
