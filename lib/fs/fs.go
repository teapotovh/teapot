// Package fs provides file system helpers and implementations.
package fs

import (
	"io"
	"os"
)

// File is a writable file.
type File interface {
	io.ReaderAt
	io.WriterAt
	Truncate(size int64) error
	Stat() (os.FileInfo, error)
	Close() error
}

// FS opens files.
type FS interface {
	OpenFile(name string, flag int, perm os.FileMode) (File, error)
}
