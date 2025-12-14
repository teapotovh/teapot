package webdav

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"strings"

	hpfs "github.com/hack-pad/hackpadfs"
	"golang.org/x/net/webdav"
)

type webDavFSWrapper struct {
	logger *slog.Logger

	fs hpfs.FS
}

func newWebDavFSWrapper(fs hpfs.FS, logger *slog.Logger) *webDavFSWrapper {
	return &webDavFSWrapper{
		logger: logger,
		fs:     fs,
	}
}

func (fsw *webDavFSWrapper) sanitizeName(name string) string {
	name = path.Clean(name)
	name = strings.TrimPrefix(name, "/")
	if name == "" {
		// Use relative indexing as required by hpfs
		name = "."
	}

	return name
}

// Mkdir implements webdav.FileSystem.
func (fsw *webDavFSWrapper) Mkdir(ctx context.Context, name string, perm os.FileMode) error {
	name = fsw.sanitizeName(name)
	fsw.logger.DebugContext(ctx, "performing FS.Mkdir", "name", name, "perm", perm)

	return hpfs.Mkdir(fsw.fs, name, perm)
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

	file, err := hpfs.OpenFile(fsw.fs, name, flag, perm)
	if err != nil {
		return nil, err
	}

	return newWebDavFileWrapper(ctx, file, fsw.logger.With("file", name)), err
}

// RemoveAll implements webdav.FileSystem.
func (fsw *webDavFSWrapper) RemoveAll(ctx context.Context, name string) error {
	name = fsw.sanitizeName(name)
	fsw.logger.DebugContext(ctx, "performing FS.RemoveAll", "name", name)

	return hpfs.RemoveAll(fsw.fs, name)
}

// Rename implements webdav.FileSystem.
func (fsw *webDavFSWrapper) Rename(ctx context.Context, oldName, newName string) error {
	oldName = fsw.sanitizeName(oldName)
	newName = fsw.sanitizeName(newName)
	fsw.logger.DebugContext(ctx, "performing FS.Rename", "old_name", oldName, "new_name", newName)

	return hpfs.Rename(fsw.fs, oldName, newName)
}

// Stat implements webdav.FileSystem.
func (fsw *webDavFSWrapper) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	name = fsw.sanitizeName(name)
	fsw.logger.DebugContext(ctx, "performing FS.Stat", "name", name)

	return hpfs.Stat(fsw.fs, name)
}

type webDavFileWrapper struct {
	logger *slog.Logger
	ctx    context.Context

	file hpfs.File
}

func newWebDavFileWrapper(ctx context.Context, file hpfs.File, logger *slog.Logger) *webDavFileWrapper {
	return &webDavFileWrapper{
		logger: logger,
		ctx:    ctx,
		file:   file,
	}
}

// Write implements io.Writer (for webdav.File).
func (fw *webDavFileWrapper) Write(p []byte) (n int, err error) {
	fw.logger.DebugContext(fw.ctx, "performing File.Write", "len(p)", len(p))
	return hpfs.WriteFile(fw.file, p)
}

// Close implements io.Closer (for http.File, for webdav.File).
func (fw *webDavFileWrapper) Close() error {
	fw.logger.DebugContext(fw.ctx, "performing File.Close")
	return fw.file.Close()
}

// Read implements io.Reader (for http.File, for webdav.File).
func (fw *webDavFileWrapper) Read(p []byte) (n int, err error) {
	fw.logger.DebugContext(fw.ctx, "performing File.Read", "len(p)", len(p))
	return fw.file.Read(p)
}

// Seek implements io.Seeker (for http.File, for webdav.File).
func (fw *webDavFileWrapper) Seek(offset int64, whence int) (int64, error) {
	fw.logger.DebugContext(fw.ctx, "performing File.Seek", "offset", offset, "whence", whence)
	return hpfs.SeekFile(fw.file, offset, whence)
}

// Readdir implements http.File (for webdav.File).
func (fw *webDavFileWrapper) Readdir(count int) ([]fs.FileInfo, error) {
	fw.logger.DebugContext(fw.ctx, "performing File.Readdir", "count", count)

	dirEntries, err := hpfs.ReadDirFile(fw.file, count)
	if err != nil {
		return nil, err
	}

	var infos []fs.FileInfo
	for _, entry := range dirEntries {
		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("error while getting file info of %s: %w", entry.Name(), err)
		}

		infos = append(infos, info)
	}
	return infos, nil
}

// Seek implements http.File (for webdav.File).
func (fw *webDavFileWrapper) Stat() (fs.FileInfo, error) {
	fw.logger.DebugContext(fw.ctx, "performing File.Stat")
	return fw.file.Stat()
}
