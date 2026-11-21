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

// Stat implements FileSystem.
func (f *FS) Stat(ctx context.Context, path string) (Inode, error) {
	inode, err := f.resolve(ctx, path)
	if err != nil {
		return Inode{}, err
	}

	return inode, nil
}

// List implements FileSystem.
func (f *FS) List(ctx context.Context, path string, cursor *Cursor) (Page, error) {
	inode, err := f.resolve(ctx, path)
	if err != nil {
		return Page{}, err
	}

	if inode.Kind != KindDirectory {
		return Page{}, ErrNotDirectory
	}

	inodes, next, err := f.store.cache.childrenPage(f.uid, inode.NodeID, cursor, PageSize)
	if err != nil {
		return Page{}, err
	}

	page := Page{Cursor: next, Inodes: make([]Inode, 0, len(inodes))}
	page.Inodes = append(page.Inodes, inodes...)

	return page, nil
}

// Open implements FileSystem.
func (f *FS) Open(ctx context.Context, path string) (File, error) {
	inode, err := f.resolve(ctx, path)
	if err != nil {
		return nil, err
	}

	if inode.Kind != KindFile {
		return nil, ErrIsDirectory
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

// Revision implements FileSystem.
func (f *FS) Revision() uint64 {
	return f.store.Revision()
}

// Put implements MutableFileSystem.
func (f *FS) Put(ctx context.Context, path string, r io.Reader, _ os.FileMode) (err error) {
	parent, name, err := splitPath(path)
	if err != nil {
		return err
	}

	parentInode, err := f.store.cache.resolve(f.uid, parent)
	if err != nil {
		return err
	}

	if parentInode.Kind != KindDirectory {
		return ErrNotDirectory
	}

	if err := f.ensureRoot(ctx); err != nil {
		return err
	}

	childID, ok, err := f.store.cache.child(f.uid, parentInode.NodeID, name)
	if err != nil {
		return err
	}

	var existing Inode
	if ok {
		existing, ok, err = f.store.cache.get(f.uid, childID)
		if err != nil {
			return err
		}

		if ok && existing.Kind == KindDirectory {
			return ErrIsDirectory
		}
	}

	key := fmt.Sprintf("%d/%s", f.uid, uuid.New().String())
	counter := &countingReader{reader: r}

	hash, err := f.store.blob.Put(ctx, key, counter)
	if err != nil {
		return err
	}

	tx, err := f.store.db.Begin(ctx, f.store.Revision())
	if err != nil {
		return err
	}

	defer rollbackOnError(ctx, tx, &err)

	inode := Inode{
		UID:      f.uid,
		ParentID: parentInode.NodeID,
		Name:     name,
		Kind:     KindFile,
		Size:     counter.count,
		ModTime:  time.Now().UTC(),
		Hash:     hash,
		S3Key:    key,
	}

	if ok {
		inode = existing
		inode.Size = counter.count
		inode.Hash = hash
		inode.S3Key = key
		inode.ModTime = time.Now().UTC()
	}

	return f.commit(ctx, tx, inode)
}

// Mkdir implements MutableFileSystem.
func (f *FS) Mkdir(ctx context.Context, path string) (err error) {
	parent, name, err := splitPath(path)
	if err != nil {
		return err
	}

	parentInode, err := f.store.cache.resolve(f.uid, parent)
	if err != nil {
		return err
	}

	if parentInode.Kind != KindDirectory {
		return ErrNotDirectory
	}

	if err := f.ensureRoot(ctx); err != nil {
		return err
	}

	if _, ok, err := f.store.cache.child(f.uid, parentInode.NodeID, name); err != nil {
		return err
	} else if ok {
		return ErrAlreadyExists
	}

	tx, err := f.store.db.Begin(ctx, f.store.Revision())
	if err != nil {
		return err
	}

	defer rollbackOnError(ctx, tx, &err)

	inode := Inode{
		UID:      f.uid,
		ParentID: parentInode.NodeID,
		Name:     name,
		Kind:     KindDirectory,
		ModTime:  time.Now().UTC(),
	}

	return f.commit(ctx, tx, inode)
}

// Delete implements MutableFileSystem.
func (f *FS) Delete(ctx context.Context, path string) (err error) {
	if _, _, err := splitPath(path); err != nil {
		return err
	}

	inode, err := f.store.cache.resolve(f.uid, path)
	if err != nil {
		return err
	}

	if inode.Kind == KindDirectory {
		children, _, err := f.store.cache.childrenPage(f.uid, inode.NodeID, nil, 1)
		if err != nil {
			return err
		}

		if len(children) > 0 {
			return ErrNotEmpty
		}
	}

	tx, err := f.store.db.Begin(ctx, f.store.Revision())
	if err != nil {
		return err
	}

	defer rollbackOnError(ctx, tx, &err)

	return f.deleteCommit(ctx, tx, inode)
}

// Rename implements MutableFileSystem.
func (f *FS) Rename(ctx context.Context, oldPath, newPath string) (err error) {
	if _, _, err := splitPath(oldPath); err != nil {
		return err
	}

	newParent, newName, err := splitPath(newPath)
	if err != nil {
		return err
	}

	inode, err := f.store.cache.resolve(f.uid, oldPath)
	if err != nil {
		return err
	}

	parentInode, err := f.store.cache.resolve(f.uid, newParent)
	if err != nil {
		return err
	}

	if parentInode.Kind != KindDirectory {
		return ErrNotDirectory
	}

	if _, ok, err := f.store.cache.child(f.uid, parentInode.NodeID, newName); err != nil {
		return err
	} else if ok {
		return ErrAlreadyExists
	}

	if err := f.checkCycle(inode, parentInode); err != nil {
		return err
	}

	tx, err := f.store.db.Begin(ctx, f.store.Revision())
	if err != nil {
		return err
	}

	defer rollbackOnError(ctx, tx, &err)

	inode.ParentID = parentInode.NodeID
	inode.Name = newName
	inode.ModTime = time.Now().UTC()

	return f.commit(ctx, tx, inode)
}

func (f *FS) ensureRoot(ctx context.Context) (err error) {
	_, ok, err := f.store.cache.get(f.uid, RootNodeID)
	if err != nil || ok {
		return err
	}

	tx, err := f.store.db.Begin(ctx, f.store.Revision())
	if err != nil {
		return err
	}

	defer rollbackOnError(ctx, tx, &err)

	return f.commit(ctx, tx, syntheticRoot(f.uid))
}

func (f *FS) commit(ctx context.Context, tx Tx, inode Inode) error {
	inode.Revision = tx.Revision()

	nodeID, err := tx.Put(ctx, inode)
	if err != nil {
		return err
	}

	inode.NodeID = nodeID

	payload, err := encodeEvent(Event{Inode: inode})
	if err != nil {
		return err
	}

	if err := tx.Commit(ctx, payload); err != nil {
		return fmt.Errorf("error while committing the write: %w", err)
	}

	return f.store.apply(ctx, Event{Inode: inode})
}

func (f *FS) deleteCommit(ctx context.Context, tx Tx, inode Inode) error {
	inode.Revision = tx.Revision()
	inode.Deleted = true
	inode.ModTime = time.Now().UTC()

	if err := tx.Delete(ctx, inode.UID, inode.NodeID); err != nil {
		return err
	}

	payload, err := encodeEvent(Event{Inode: inode})
	if err != nil {
		return err
	}

	if err := tx.Commit(ctx, payload); err != nil {
		return fmt.Errorf("error while committing the delete: %w", err)
	}

	return f.store.apply(ctx, Event{Inode: inode})
}

func (f *FS) resolve(_ context.Context, path string) (Inode, error) {
	return f.store.cache.resolve(f.uid, path)
}

func (f *FS) checkCycle(inode, parent Inode) error {
	if inode.Kind != KindDirectory {
		return nil
	}

	current := parent
	for range maxDepth {
		if current.NodeID == inode.NodeID {
			return ErrInvalidPath
		}

		if current.NodeID == RootNodeID || current.ParentID == current.NodeID {
			return nil
		}

		next, ok, err := f.store.cache.get(inode.UID, current.ParentID)
		if err != nil {
			return err
		}

		if !ok {
			return nil
		}

		current = next
	}

	return ErrInvalidPath
}

func splitPath(path string) (string, string, error) {
	clean, err := CleanPath(path)
	if err != nil {
		return "", "", err
	}

	if clean == "/" {
		return "", "", ErrInvalidPath
	}

	index := strings.LastIndexByte(clean, '/')

	parent := clean[:index]
	if parent == "" {
		parent = "/"
	}

	name, err := NormalizeName(clean[index+1:])
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

func (f *file) Stat() (Inode, error) {
	return f.inode, nil
}

// Ensure FS implements the file interfaces.
var _ MutableFileSystem = (*FS)(nil)
