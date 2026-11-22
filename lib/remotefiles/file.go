package remotefiles

import (
	"io"
)

type openFile struct {
	io.ReadSeekCloser

	inode Inode
}

// Stat implements File.
func (f *openFile) Stat() (Inode, error) {
	return f.inode, nil
}

// Ensure openFile implements File.
var _ File = (*openFile)(nil)
