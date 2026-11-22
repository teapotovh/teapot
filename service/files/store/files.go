package store

import (
	"context"
	"errors"
	"io"

	"github.com/teapotovh/teapot/lib/s3cache"
)

var (
	// ErrNotFound reports that a path does not exist.
	ErrNotFound = errors.New("not found")
	// ErrNotDirectory reports that a path is not a directory.
	ErrNotDirectory = errors.New("not a directory")
	// ErrIsDirectory reports that a path is a directory.
	ErrIsDirectory = errors.New("is a directory")
	// ErrInvalidPath reports that a path is not valid.
	ErrInvalidPath = errors.New("invalid path")
	// ErrInvalidName reports that a name is not valid.
	ErrInvalidName = errors.New("invalid name")
	// ErrReadOnly reports a write on a read-only file system.
	ErrReadOnly = errors.New("file system is read-only")
)

// FileSystem is the read-only file interface for one user.
type FileSystem interface {
	// Stat returns the inode at path.
	Stat(ctx context.Context, path string) (Inode, error)
	// List returns the children of path after cursor. A nil cursor starts at the first page.
	List(ctx context.Context, path string, cursor *Cursor) (Page, error)
	// Open opens the file at path.
	Open(ctx context.Context, path string) (File, error)
	// Revision returns the current revision of the view.
	Revision() uint64
}

// MutableFileSystem is a file interface that also allows writes.
type MutableFileSystem interface {
	FileSystem
	// Put writes the content of r to path.
	Put(ctx context.Context, path string, r io.Reader) error
	// Delete removes path.
	Delete(ctx context.Context, path string) error
	// Rename moves oldPath to newPath.
	Rename(ctx context.Context, oldPath, newPath string) error
	// Mkdir creates a directory at path.
	Mkdir(ctx context.Context, path string) error
}

// File is an open file.
type File interface {
	io.Reader
	io.Seeker
	io.Closer
	// Stat returns the inode of the file.
	Stat() (Inode, error)
}

// Hash is the S3 entity tag of a blob.
type Hash = s3cache.Hash

// Kind is the type of an inode.
type Kind uint8

const (
	// KindDirectory marks a directory.
	KindDirectory Kind = iota
	// KindFile marks a file.
	KindFile
)

// Cursor is an opaque position in a list.
type Cursor string

// Page is one page of inodes.
type Page struct {
	// Inodes are the inodes in this page.
	Inodes []Inode
	// Cursor is the cursor of the next page. A nil cursor means the last page.
	Cursor *Cursor
}
