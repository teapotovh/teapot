package fs

import "os"

// OS is the operating system file system.
type OS struct{}

// OpenFile implements FS.
func (OS) OpenFile(name string, flag int, perm os.FileMode) (File, error) {
	return os.OpenFile(name, flag, perm) //nolint:gosec // The caller controls the path.
}

// Ensure OS implements FS.
var _ FS = OS{}
