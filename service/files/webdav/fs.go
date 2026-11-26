package webdav

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path"

	libwebdav "github.com/teapotovh/teapot/lib/webdav"
	"github.com/teapotovh/teapot/service/files/store"
)

type fileSystem struct {
	fs *store.FS
}

func newFileSystem(fs *store.FS) *fileSystem {
	return &fileSystem{fs: fs}
}

// Open implements webdav.FileSystem.
func (f *fileSystem) Open(ctx context.Context, name string) (io.ReadCloser, error) {
	file, err := f.fs.Open(ctx, name)
	if err != nil {
		return nil, storeError(err)
	}

	return file, nil
}

// Stat implements webdav.FileSystem.
func (f *fileSystem) Stat(ctx context.Context, name string) (*libwebdav.FileInfo, error) {
	inode, err := f.fs.Stat(ctx, name)
	if err != nil {
		return nil, storeError(err)
	}

	return fileInfo(inode, name), nil
}

// ReadDir implements webdav.FileSystem.
func (f *fileSystem) ReadDir(
	ctx context.Context,
	name string,
	recursive bool,
) ([]libwebdav.FileInfo, error) {
	root, err := f.Stat(ctx, name)
	if err != nil {
		return nil, err
	}

	infos := []libwebdav.FileInfo{*root}

	if err := f.readDir(ctx, path.Clean(name), recursive, &infos); err != nil {
		return nil, err
	}

	return infos, nil
}

// Create implements webdav.FileSystem.
func (f *fileSystem) Create(
	ctx context.Context,
	name string,
	body io.ReadCloser,
	options *libwebdav.CreateOptions,
) (*libwebdav.FileInfo, bool, error) {
	inode, err := f.fs.Stat(ctx, name)
	exists := err == nil

	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, false, storeError(err)
	}

	var current *libwebdav.FileInfo

	if exists {
		if inode.Kind == store.KindDirectory {
			return nil, false, libwebdav.NewHTTPError(http.StatusMethodNotAllowed, store.ErrIsDirectory)
		}

		current = fileInfo(inode, name)
	}

	pre := libwebdav.Preconditions{IfMatch: options.IfMatch, IfNoneMatch: options.IfNoneMatch}

	var etag string
	if current != nil {
		etag = current.ETag
	}

	if err := pre.CheckWrite(exists, etag); err != nil {
		return nil, false, libwebdav.NewHTTPError(http.StatusPreconditionFailed, err)
	}

	if err := f.fs.Put(ctx, name, body); err != nil {
		return nil, false, storeError(err)
	}

	info, err := f.Stat(ctx, name)
	if err != nil {
		return nil, false, err
	}

	return info, !exists, nil
}

// RemoveAll implements webdav.FileSystem.
func (f *fileSystem) RemoveAll(
	ctx context.Context,
	name string,
	options *libwebdav.RemoveAllOptions,
) error {
	info, err := f.Stat(ctx, name)
	if err != nil {
		return err
	}

	pre := libwebdav.Preconditions{IfMatch: options.IfMatch, IfNoneMatch: options.IfNoneMatch}
	if err := pre.CheckWrite(true, info.ETag); err != nil {
		return libwebdav.NewHTTPError(http.StatusPreconditionFailed, err)
	}

	return f.removeAll(ctx, name)
}

// Mkdir implements webdav.FileSystem.
func (f *fileSystem) Mkdir(ctx context.Context, name string) error {
	if err := f.fs.Mkdir(ctx, name); err != nil {
		return storeError(err)
	}

	return nil
}

// Copy implements webdav.FileSystem.
func (f *fileSystem) Copy(
	ctx context.Context,
	name string,
	dest string,
	options *libwebdav.CopyOptions,
) (bool, error) {
	src, err := f.fs.Stat(ctx, name)
	if err != nil {
		return false, storeError(err)
	}

	created := false

	if _, err := f.fs.Stat(ctx, dest); err == nil {
		if options.NoOverwrite {
			return false, libwebdav.NewHTTPError(http.StatusPreconditionFailed, os.ErrExist)
		}

		if err := f.removeAll(ctx, dest); err != nil {
			return false, err
		}
	} else if errors.Is(err, store.ErrNotFound) {
		created = true
	} else {
		return false, storeError(err)
	}

	if err := f.copy(ctx, src, name, dest, options.NoRecursive); err != nil {
		return false, err
	}

	return created, nil
}

// Move implements webdav.FileSystem.
func (f *fileSystem) Move(
	ctx context.Context,
	name string,
	dest string,
	options *libwebdav.MoveOptions,
) (bool, error) {
	if _, err := f.fs.Stat(ctx, name); err != nil {
		return false, storeError(err)
	}

	created := false

	if _, err := f.fs.Stat(ctx, dest); err == nil {
		if options.NoOverwrite {
			return false, libwebdav.NewHTTPError(http.StatusPreconditionFailed, os.ErrExist)
		}

		if err := f.removeAll(ctx, dest); err != nil {
			return false, err
		}
	} else if errors.Is(err, store.ErrNotFound) {
		created = true
	} else {
		return false, storeError(err)
	}

	if err := f.fs.Rename(ctx, name, dest); err != nil {
		return false, storeError(err)
	}

	return created, nil
}

func (f *fileSystem) readDir(
	ctx context.Context,
	name string,
	recursive bool,
	infos *[]libwebdav.FileInfo,
) error {
	var cursor *store.Cursor

	for {
		page, err := f.fs.List(ctx, name, cursor)
		if err != nil {
			return storeError(err)
		}

		for _, inode := range page.Inodes {
			child := path.Join(name, inode.Name)
			*infos = append(*infos, *fileInfo(inode, child))

			if recursive && inode.Kind == store.KindDirectory {
				if err := f.readDir(ctx, child, recursive, infos); err != nil {
					return err
				}
			}
		}

		if page.Cursor == nil {
			return nil
		}

		cursor = page.Cursor
	}
}

func (f *fileSystem) removeAll(ctx context.Context, name string) error {
	inode, err := f.fs.Stat(ctx, name)
	if err != nil {
		return storeError(err)
	}

	if inode.Kind == store.KindDirectory {
		var cursor *store.Cursor

		for {
			page, err := f.fs.List(ctx, name, cursor)
			if err != nil {
				return storeError(err)
			}

			for _, child := range page.Inodes {
				if err := f.removeAll(ctx, path.Join(name, child.Name)); err != nil {
					return err
				}
			}

			if page.Cursor == nil {
				break
			}

			cursor = page.Cursor
		}
	}

	if err := f.fs.Delete(ctx, name); err != nil {
		return storeError(err)
	}

	return nil
}

func (f *fileSystem) copy(
	ctx context.Context,
	inode store.Inode,
	src string,
	dest string,
	noRecursive bool,
) error {
	if inode.Kind == store.KindDirectory {
		if err := f.fs.Mkdir(ctx, dest); err != nil {
			return storeError(err)
		}

		if noRecursive {
			return nil
		}

		var cursor *store.Cursor

		for {
			page, err := f.fs.List(ctx, src, cursor)
			if err != nil {
				return storeError(err)
			}

			for _, child := range page.Inodes {
				if err := f.copy(
					ctx,
					child,
					path.Join(src, child.Name),
					path.Join(dest, child.Name),
					false,
				); err != nil {
					return err
				}
			}

			if page.Cursor == nil {
				return nil
			}

			cursor = page.Cursor
		}
	}

	file, err := f.fs.Open(ctx, src)
	if err != nil {
		return storeError(err)
	}

	if err := f.fs.Put(ctx, dest, file); err != nil {
		return errors.Join(storeError(err), file.Close())
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf("error while closing %q: %w", src, err)
	}

	return nil
}

func fileInfo(inode store.Inode, name string) *libwebdav.FileInfo {
	info := &libwebdav.FileInfo{
		Path:    name,
		Size:    inode.Size,
		ModTime: inode.ModTime,
		IsDir:   inode.Kind == store.KindDirectory,
	}

	if inode.Kind == store.KindFile {
		info.MIMEType = mime.TypeByExtension(path.Ext(inode.Name))
		info.ETag = string(inode.Hash)
	}

	return info
}

func storeError(err error) error {
	switch {
	case errors.Is(err, store.ErrNotFound):
		return libwebdav.NewHTTPError(http.StatusNotFound, err)
	case errors.Is(err, store.ErrQuotaExceeded):
		return libwebdav.NewHTTPError(http.StatusInsufficientStorage, err)
	case errors.Is(err, store.ErrAlreadyExists):
		return libwebdav.NewHTTPError(http.StatusMethodNotAllowed, err)
	case errors.Is(err, store.ErrNotEmpty):
		return libwebdav.NewHTTPError(http.StatusConflict, err)
	case errors.Is(err, store.ErrNotDirectory), errors.Is(err, store.ErrIsDirectory):
		return libwebdav.NewHTTPError(http.StatusConflict, err)
	case errors.Is(err, store.ErrInvalidPath), errors.Is(err, store.ErrInvalidName):
		return libwebdav.NewHTTPError(http.StatusBadRequest, err)
	default:
		return err
	}
}

// Ensure fileSystem implements webdav.FileSystem.
var _ libwebdav.FileSystem = (*fileSystem)(nil)
