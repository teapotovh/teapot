package remotefiles

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/teapotovh/teapot/lib/fs"
)

var (
	// ErrNotFound reports that a path does not exist.
	ErrNotFound = errors.New("not found")
	// ErrNotDirectory reports that a path is not a directory.
	ErrNotDirectory = errors.New("not a directory")
	// ErrIsDirectory reports that a path is a directory.
	ErrIsDirectory = errors.New("is a directory")
	// ErrInvalidPath reports that a path is not valid.
	ErrInvalidPath = fs.ErrInvalidPath
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

// File is an open file.
type File interface {
	io.Reader
	io.Seeker
	io.Closer
	// Stat returns the inode of the file.
	Stat() (Inode, error)
}

// Hash is the S3 entity tag of a blob.
type Hash string

// Kind is the type of an inode.
type Kind uint8

const (
	// KindDirectory marks a directory.
	KindDirectory Kind = iota
	// KindFile marks a file.
	KindFile
)

// Inode is a file or a directory.
type Inode struct {
	// UID is the id of the user that owns the inode.
	UID int64
	// NodeID is the id of the inode in the user tree.
	NodeID uint64
	// ParentID is the id of the parent inode. The root is its own parent.
	ParentID uint64
	// Name is the last element of the path.
	Name string
	// Kind is the type of the inode.
	Kind Kind
	// Size is the size of the file in bytes.
	Size int64
	// ModTime is the time of the last change.
	ModTime time.Time
	// Hash is the S3 entity tag of the current blob.
	Hash Hash
	// S3Key is the S3 object key of the current blob.
	S3Key string
	// Revision is the revision of the last change.
	Revision uint64
}

// Cursor is an opaque position in a list.
type Cursor string

// Page is one page of inodes.
type Page struct {
	// Inodes are the inodes in this page.
	Inodes []Inode
	// Cursor is the cursor of the next page. A nil cursor means the last page.
	Cursor *Cursor
}
