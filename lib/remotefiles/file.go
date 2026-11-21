package remotefiles

import (
	"bytes"

	"github.com/teapotovh/teapot/lib/files"
)

type openFile struct {
	*bytes.Reader

	inode files.Inode
}

// Stat implements files.File.
func (f *openFile) Stat() (files.Inode, error) {
	return f.inode, nil
}

// Close implements files.File.
func (f *openFile) Close() error {
	return nil
}

var _ files.File = (*openFile)(nil)
