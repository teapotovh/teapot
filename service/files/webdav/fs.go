package webdav

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"strings"
	"time"

	"golang.org/x/net/webdav"

	"github.com/teapotovh/teapot/service/files/store"
)

type webDavFSWrapper struct {
	logger *slog.Logger

	fs *store.FS
}

func newWebDavFSWrapper(fs *store.FS, logger *slog.Logger) *webDavFSWrapper {
	return &webDavFSWrapper{
		logger: logger,
		fs:     fs,
	}
}

// Mkdir implements webdav.FileSystem.
func (fsw *webDavFSWrapper) Mkdir(ctx context.Context, name string, _ os.FileMode) error {
	name = fsw.sanitizeName(name)
	fsw.logger.DebugContext(ctx, "performing FS.Mkdir", "name", name)

	if err := fsw.fs.Mkdir(ctx, name); err != nil {
		return fmt.Errorf("could not create directory %q: %w", name, err)
	}

	return nil
}

// OpenFile implements webdav.FileSystem.
func (fsw *webDavFSWrapper) OpenFile(
	ctx context.Context,
	name string,
	flag int,
	perm os.FileMode,
) (webdav.File, error) {
	name = fsw.sanitizeName(name)
	fsw.logger.DebugContext(ctx, "performing FS.OpenFile", "name", name, "flag", flag, "perm", perm)

	file := &webDavFileWrapper{
		logger: fsw.logger.With("file", name),
		ctx:    ctx,
		fs:     fsw.fs,
		name:   name,
	}

	if flag&(os.O_WRONLY|os.O_RDWR|os.O_CREATE|os.O_TRUNC|os.O_APPEND) != 0 {
		file.writable = true
		file.perm = perm
		file.buffer = &bytes.Buffer{}

		if flag&os.O_TRUNC == 0 {
			data, err := fsw.readFile(ctx, name)
			if err != nil && !errors.Is(err, store.ErrNotFound) {
				return nil, err
			}

			file.buffer.Write(data)
		}

		return file, nil
	}

	inode, err := fsw.fs.Stat(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("could not stat %q: %w", name, err)
	}

	if inode.Kind == store.KindDirectory {
		file.directory = true

		return file, nil
	}

	opened, err := fsw.fs.Open(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("could not open %q: %w", name, err)
	}

	file.file = opened

	return file, nil
}

// RemoveAll implements webdav.FileSystem.
func (fsw *webDavFSWrapper) RemoveAll(ctx context.Context, name string) error {
	name = fsw.sanitizeName(name)
	fsw.logger.DebugContext(ctx, "performing FS.RemoveAll", "name", name)

	if err := fsw.removeAll(ctx, name); err != nil {
		return fmt.Errorf("could not remove %q: %w", name, err)
	}

	return nil
}

// Rename implements webdav.FileSystem.
func (fsw *webDavFSWrapper) Rename(ctx context.Context, oldName, newName string) error {
	oldName = fsw.sanitizeName(oldName)
	newName = fsw.sanitizeName(newName)
	fsw.logger.DebugContext(ctx, "performing FS.Rename", "old_name", oldName, "new_name", newName)

	if err := fsw.fs.Rename(ctx, oldName, newName); err != nil {
		return fmt.Errorf("could not rename %q to %q: %w", oldName, newName, err)
	}

	return nil
}

// Stat implements webdav.FileSystem.
func (fsw *webDavFSWrapper) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	name = fsw.sanitizeName(name)
	fsw.logger.DebugContext(ctx, "performing FS.Stat", "name", name)

	inode, err := fsw.fs.Stat(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("could not stat %q: %w", name, err)
	}

	return newFileInfo(inode), nil
}

func (fsw *webDavFSWrapper) sanitizeName(name string) string {
	name = path.Clean("/" + strings.TrimPrefix(name, "/"))

	return name
}

func (fsw *webDavFSWrapper) readFile(ctx context.Context, name string) (data []byte, err error) {
	file, err := fsw.fs.Open(ctx, name)
	if err != nil {
		return nil, err
	}

	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			err = errors.Join(fmt.Errorf("could not close %q: %w", name, closeErr), err)
		}
	}()

	data, err = io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("could not read %q: %w", name, err)
	}

	return data, nil
}

func (fsw *webDavFSWrapper) removeAll(ctx context.Context, name string) error {
	inode, err := fsw.fs.Stat(ctx, name)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}

		return err
	}

	if inode.Kind == store.KindDirectory {
		var cursor *store.Cursor

		for {
			page, err := fsw.fs.List(ctx, name, cursor)
			if err != nil {
				return err
			}

			for _, child := range page.Inodes {
				childPath := path.Join(name, child.Name)
				if err := fsw.removeAll(ctx, childPath); err != nil {
					return err
				}
			}

			if page.Cursor == nil {
				break
			}

			cursor = page.Cursor
		}
	}

	return fsw.fs.Delete(ctx, name)
}

type webDavFileWrapper struct {
	logger *slog.Logger
	ctx    context.Context

	fs   *store.FS
	name string

	file      store.File
	buffer    *bytes.Buffer
	writable  bool
	directory bool
	perm      os.FileMode
}

// Write implements io.Writer.
func (fw *webDavFileWrapper) Write(p []byte) (int, error) {
	fw.logger.DebugContext(fw.ctx, "performing File.Write", "len(p)", len(p))

	if !fw.writable {
		return 0, fmt.Errorf("could not write to %q: %w", fw.name, fs.ErrPermission)
	}

	return fw.buffer.Write(p)
}

// Close implements io.Closer.
func (fw *webDavFileWrapper) Close() (err error) {
	fw.logger.DebugContext(fw.ctx, "performing File.Close")

	if fw.writable {
		if err := fw.fs.Put(fw.ctx, fw.name, bytes.NewReader(fw.buffer.Bytes())); err != nil {
			return fmt.Errorf("could not store %q: %w", fw.name, err)
		}
	}

	if fw.file != nil {
		if closeErr := fw.file.Close(); closeErr != nil {
			err = fmt.Errorf("could not close %q: %w", fw.name, closeErr)
		}

		fw.file = nil
	}

	return err
}

// Read implements io.Reader.
func (fw *webDavFileWrapper) Read(p []byte) (int, error) {
	fw.logger.DebugContext(fw.ctx, "performing File.Read", "len(p)", len(p))

	if fw.file == nil {
		return 0, fmt.Errorf("could not read %q: %w", fw.name, fs.ErrInvalid)
	}

	return fw.file.Read(p)
}

// Seek implements io.Seeker.
func (fw *webDavFileWrapper) Seek(offset int64, whence int) (int64, error) {
	fw.logger.DebugContext(fw.ctx, "performing File.Seek", "offset", offset, "whence", whence)

	if fw.file == nil {
		return 0, fmt.Errorf("could not seek %q: %w", fw.name, fs.ErrInvalid)
	}

	return fw.file.Seek(offset, whence)
}

// Readdir implements http.File.
func (fw *webDavFileWrapper) Readdir(count int) ([]fs.FileInfo, error) {
	fw.logger.DebugContext(fw.ctx, "performing File.Readdir", "count", count)

	if !fw.directory {
		return nil, fmt.Errorf("could not read directory %q: %w", fw.name, fs.ErrInvalid)
	}

	var (
		infos  []fs.FileInfo
		cursor *store.Cursor
	)

	for {
		page, err := fw.fs.List(fw.ctx, fw.name, cursor)
		if err != nil {
			return nil, fmt.Errorf("could not list %q: %w", fw.name, err)
		}

		for _, inode := range page.Inodes {
			infos = append(infos, newFileInfo(inode))

			if count > 0 && len(infos) == count {
				return infos, nil
			}
		}

		if page.Cursor == nil {
			break
		}

		cursor = page.Cursor
	}

	return infos, nil
}

// Stat implements http.File.
func (fw *webDavFileWrapper) Stat() (fs.FileInfo, error) {
	fw.logger.DebugContext(fw.ctx, "performing File.Stat")

	if fw.file != nil {
		inode, err := fw.file.Stat()
		if err != nil {
			return nil, fmt.Errorf("could not stat %q: %w", fw.name, err)
		}

		return newFileInfo(inode), nil
	}

	inode, err := fw.fs.Stat(fw.ctx, fw.name)
	if err != nil {
		return nil, fmt.Errorf("could not stat %q: %w", fw.name, err)
	}

	return newFileInfo(inode), nil
}

type fileInfo struct {
	inode store.Inode
}

func newFileInfo(inode store.Inode) fileInfo {
	return fileInfo{inode: inode}
}

func (i fileInfo) Name() string {
	name := path.Base(i.inode.Name)
	if name == "." {
		return i.inode.Name
	}

	return name
}

func (i fileInfo) Size() int64        { return i.inode.Size }
func (i fileInfo) ModTime() time.Time { return i.inode.ModTime }
func (i fileInfo) IsDir() bool        { return i.inode.Kind == store.KindDirectory }
func (i fileInfo) Sys() any           { return nil }

func (i fileInfo) Mode() os.FileMode {
	if i.IsDir() {
		return os.ModeDir | 0o0750
	}

	return 0o0640
}
