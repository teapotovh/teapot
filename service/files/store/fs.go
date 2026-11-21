package store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"uuid"

	"github.com/teapotovh/teapot/lib/files"
)

var (
	// ErrAlreadyExists reports that a path already exists.
	ErrAlreadyExists = errors.New("already exists")
	// ErrNotEmpty reports a delete on a directory with children.
	ErrNotEmpty = errors.New("directory is not empty")
)

// FS is the file system view for one user.
type FS struct {
	store *Store
	uid   int64
}

// Stat implements files.FS.
func (f *FS) Stat(ctx context.Context, path string) (files.Inode, error) {
	inode, err := f.resolve(ctx, path)
	if err != nil {
		return files.Inode{}, err
	}

	return inode.toFiles(), nil
}

// List implements files.FS.
func (f *FS) List(ctx context.Context, path string, cursor *files.Cursor) (files.Page, error) {
	inode, err := f.resolve(ctx, path)
	if err != nil {
		return files.Page{}, err
	}

	if inode.Kind != files.KindDirectory {
		return files.Page{}, files.ErrNotDirectory
	}

	inodes, next, err := f.store.cache.childrenPage(f.uid, inode.NodeID, cursor, PageSize)
	if err != nil {
		return files.Page{}, err
	}

	if len(inodes) == 0 && cursor == nil {
		inodes, next, err = f.loadChildren(ctx, inode.NodeID)
		if err != nil {
			return files.Page{}, err
		}
	}

	page := files.Page{Cursor: next, Inodes: make([]files.Inode, 0, len(inodes))}
	for _, child := range inodes {
		page.Inodes = append(page.Inodes, child.toFiles())
	}

	return page, nil
}

// Open implements files.FS.
func (f *FS) Open(ctx context.Context, path string) (files.File, error) {
	inode, err := f.resolve(ctx, path)
	if err != nil {
		return nil, err
	}

	if inode.Kind != files.KindFile {
		return nil, files.ErrIsDirectory
	}

	if inode.S3Key == "" {
		return &file{reader: bytes.NewReader(nil), inode: inode}, nil
	}

	reader, err := f.store.blob.Get(ctx, inode.S3Key)
	if err != nil {
		return nil, err
	}

	return &file{reader: reader, closer: reader, inode: inode}, nil
}

// Revision implements files.FS.
func (f *FS) Revision() uint64 {
	return f.store.Revision()
}

// Put implements files.MutableFS.
func (f *FS) Put(ctx context.Context, path string, r io.Reader, _ os.FileMode) error {
	parent, name, err := splitPath(path)
	if err != nil {
		return err
	}

	key := fmt.Sprintf("%d/%s", f.uid, uuid.New().String())
	counter := &countingReader{reader: r}

	hash, err := f.store.blob.Put(ctx, key, counter)
	if err != nil {
		return err
	}

	tx, err := f.store.db.Begin(ctx)
	if err != nil {
		return err
	}

	defer func() { _ = tx.Rollback(ctx) }()

	if err := f.ensureRoot(ctx, tx); err != nil {
		return err
	}

	revision, err := tx.BumpRevision(ctx)
	if err != nil {
		return err
	}

	parentInode, err := resolveTx(ctx, tx, f.uid, parent)
	if err != nil {
		return err
	}

	if parentInode.Kind != files.KindDirectory {
		return files.ErrNotDirectory
	}

	existing, ok, err := tx.Child(ctx, f.uid, parentInode.NodeID, name)
	if err != nil {
		return err
	}

	if ok && existing.Kind == files.KindDirectory {
		return files.ErrIsDirectory
	}

	inode, err := f.fileInode(ctx, tx, revision, parentInode.NodeID, name, hash, key, counter.count, existing, ok)
	if err != nil {
		return err
	}

	return f.commit(ctx, tx, inode)
}

// Mkdir implements files.MutableFS.
func (f *FS) Mkdir(ctx context.Context, path string) error {
	parent, name, err := splitPath(path)
	if err != nil {
		return err
	}

	tx, err := f.store.db.Begin(ctx)
	if err != nil {
		return err
	}

	defer func() { _ = tx.Rollback(ctx) }()

	if err := f.ensureRoot(ctx, tx); err != nil {
		return err
	}

	revision, err := tx.BumpRevision(ctx)
	if err != nil {
		return err
	}

	parentInode, err := resolveTx(ctx, tx, f.uid, parent)
	if err != nil {
		return err
	}

	if parentInode.Kind != files.KindDirectory {
		return files.ErrNotDirectory
	}

	if _, ok, err := tx.Child(ctx, f.uid, parentInode.NodeID, name); err != nil {
		return err
	} else if ok {
		return ErrAlreadyExists
	}

	nodeID, err := tx.NextNodeID(ctx, f.uid)
	if err != nil {
		return err
	}

	inode := Inode{
		UID:      f.uid,
		NodeID:   nodeID,
		ParentID: parentInode.NodeID,
		Name:     name,
		Kind:     files.KindDirectory,
		ModTime:  time.Now().UTC(),
		Revision: revision,
	}

	return f.commit(ctx, tx, inode)
}

// Delete implements files.MutableFS.
func (f *FS) Delete(ctx context.Context, path string) error {
	_, name, err := splitPath(path)
	if err != nil {
		return err
	}

	tx, err := f.store.db.Begin(ctx)
	if err != nil {
		return err
	}

	defer func() { _ = tx.Rollback(ctx) }()

	revision, err := tx.BumpRevision(ctx)
	if err != nil {
		return err
	}

	inode, err := resolveTx(ctx, tx, f.uid, path)
	if err != nil {
		return err
	}

	if inode.Kind == files.KindDirectory {
		children, err := tx.Children(ctx, f.uid, inode.NodeID)
		if err != nil {
			return err
		}

		if len(children) > 0 {
			return ErrNotEmpty
		}
	}

	inode.Name = name
	inode.Deleted = true
	inode.ModTime = time.Now().UTC()
	inode.Revision = revision

	return f.commit(ctx, tx, inode)
}

// Rename implements files.MutableFS.
func (f *FS) Rename(ctx context.Context, oldPath, newPath string) error {
	if _, _, err := splitPath(oldPath); err != nil {
		return err
	}

	newParent, newName, err := splitPath(newPath)
	if err != nil {
		return err
	}

	tx, err := f.store.db.Begin(ctx)
	if err != nil {
		return err
	}

	defer func() { _ = tx.Rollback(ctx) }()

	revision, err := tx.BumpRevision(ctx)
	if err != nil {
		return err
	}

	inode, err := resolveTx(ctx, tx, f.uid, oldPath)
	if err != nil {
		return err
	}

	parentInode, err := resolveTx(ctx, tx, f.uid, newParent)
	if err != nil {
		return err
	}

	if parentInode.Kind != files.KindDirectory {
		return files.ErrNotDirectory
	}

	if _, ok, err := tx.Child(ctx, f.uid, parentInode.NodeID, newName); err != nil {
		return err
	} else if ok {
		return ErrAlreadyExists
	}

	if err := checkCycle(ctx, tx, inode, parentInode); err != nil {
		return err
	}

	inode.ParentID = parentInode.NodeID
	inode.Name = newName
	inode.ModTime = time.Now().UTC()
	inode.Revision = revision

	return f.commit(ctx, tx, inode)
}

func (f *FS) ensureRoot(ctx context.Context, tx Tx) error {
	_, ok, err := tx.Inode(ctx, f.uid, RootNodeID)
	if err != nil {
		return err
	}

	if ok {
		return nil
	}

	revision, err := tx.BumpRevision(ctx)
	if err != nil {
		return err
	}

	return tx.EnsureRoot(ctx, f.uid, revision)
}

func (f *FS) fileInode(
	ctx context.Context,
	tx Tx,
	revision uint64,
	parentID uint64,
	name string,
	hash files.Hash,
	key string,
	size int64,
	existing Inode,
	ok bool,
) (Inode, error) {
	if ok {
		existing.Size = size
		existing.Hash = hash
		existing.S3Key = key
		existing.ModTime = time.Now().UTC()
		existing.Revision = revision

		return existing, nil
	}

	nodeID, err := tx.NextNodeID(ctx, f.uid)
	if err != nil {
		return Inode{}, err
	}

	return Inode{
		UID:      f.uid,
		NodeID:   nodeID,
		ParentID: parentID,
		Name:     name,
		Kind:     files.KindFile,
		Size:     size,
		ModTime:  time.Now().UTC(),
		Hash:     hash,
		S3Key:    key,
		Revision: revision,
	}, nil
}

func (f *FS) commit(ctx context.Context, tx Tx, inode Inode) error {
	payload, err := encodeEvent(Event{Inode: inode})
	if err != nil {
		return err
	}

	if err := tx.Upsert(ctx, inode); err != nil {
		return err
	}

	if err := tx.Notify(ctx, payload); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("error while committing the write: %w", err)
	}

	return f.store.apply(ctx, Event{Inode: inode})
}

func (f *FS) resolve(ctx context.Context, path string) (Inode, error) {
	inode, err := f.store.cache.resolve(f.uid, path)
	if err == nil {
		return inode, nil
	}

	if !errors.Is(err, files.ErrNotFound) {
		return Inode{}, err
	}

	return f.resolveDB(ctx, path)
}

func (f *FS) resolveDB(ctx context.Context, path string) (Inode, error) {
	tx, err := f.store.db.Begin(ctx)
	if err != nil {
		return Inode{}, err
	}

	defer func() { _ = tx.Rollback(ctx) }()

	return resolveTx(ctx, tx, f.uid, path)
}

func (f *FS) loadChildren(ctx context.Context, parentID uint64) ([]Inode, *files.Cursor, error) {
	tx, err := f.store.db.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}

	defer func() { _ = tx.Rollback(ctx) }()

	children, err := tx.Children(ctx, f.uid, parentID)
	if err != nil {
		return nil, nil, err
	}

	for _, child := range children {
		if err := f.store.cache.apply(child); err != nil {
			return nil, nil, err
		}
	}

	return f.store.cache.childrenPage(f.uid, parentID, nil, PageSize)
}

func resolveTx(ctx context.Context, tx Tx, uid int64, path string) (Inode, error) {
	clean, err := files.CleanPath(path)
	if err != nil {
		return Inode{}, err
	}

	if clean == "/" {
		inode, ok, err := tx.Inode(ctx, uid, RootNodeID)
		if err != nil {
			return Inode{}, err
		}

		if ok {
			return inode, nil
		}

		return syntheticRoot(uid), nil
	}

	current, err := resolveTx(ctx, tx, uid, "/")
	if err != nil {
		return Inode{}, err
	}

	for name := range strings.SplitSeq(strings.TrimPrefix(clean, "/"), "/") {
		inode, ok, err := tx.Child(ctx, uid, current.NodeID, name)
		if err != nil {
			return Inode{}, err
		}

		if !ok {
			return Inode{}, files.ErrNotFound
		}

		current = inode
	}

	return current, nil
}

func checkCycle(ctx context.Context, tx Tx, inode, parent Inode) error {
	if inode.Kind != files.KindDirectory {
		return nil
	}

	current := parent
	for range maxDepth {
		if current.NodeID == inode.NodeID {
			return files.ErrInvalidPath
		}

		if current.NodeID == RootNodeID || current.ParentID == current.NodeID {
			return nil
		}

		next, ok, err := tx.Inode(ctx, inode.UID, current.ParentID)
		if err != nil {
			return err
		}

		if !ok {
			return nil
		}

		current = next
	}

	return files.ErrInvalidPath
}

func splitPath(path string) (string, string, error) {
	clean, err := files.CleanPath(path)
	if err != nil {
		return "", "", err
	}

	if clean == "/" {
		return "", "", files.ErrInvalidPath
	}

	index := strings.LastIndexByte(clean, '/')

	parent := clean[:index]
	if parent == "" {
		parent = "/"
	}

	name, err := files.NormalizeName(clean[index+1:])
	if err != nil {
		return "", "", err
	}

	return parent, name, nil
}

type countingReader struct {
	reader io.Reader
	count  int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.reader.Read(p)
	c.count += int64(n)

	return n, err
}

type file struct {
	reader io.ReadSeeker
	closer io.Closer
	inode  Inode
}

func (f *file) Read(p []byte) (int, error) {
	return f.reader.Read(p)
}

func (f *file) Seek(offset int64, whence int) (int64, error) {
	return f.reader.Seek(offset, whence)
}

func (f *file) Close() error {
	if f.closer == nil {
		return nil
	}

	return f.closer.Close()
}

func (f *file) Stat() (files.Inode, error) {
	return f.inode.toFiles(), nil
}

// Ensure FS implements the file interfaces.
var _ files.MutableFS = (*FS)(nil)
