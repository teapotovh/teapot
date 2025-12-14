package files

import (
	"errors"

	hpfs "github.com/hack-pad/hackpadfs"
)

var ErrReadOnly = errors.New("filesystem is read-only")

type ReadOnlyFS[FS hpfs.FS] struct {
	fs FS
}

// NewReadOnlyFS returns a new read-only FS which proxies all calls to the
// underlying FS, and returns an error for any write call.
func NewReadOnlyFS[FS hpfs.FS](fs FS) ReadOnlyFS[FS] {
	return ReadOnlyFS[FS]{fs}
}

// Open implements hackpadfs.FS.
func (fs ReadOnlyFS[FS]) Open(name string) (hpfs.File, error) {
	return fs.fs.Open(name)
}
