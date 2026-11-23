package web

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"path"

	"github.com/teapotovh/teapot/lib/webhandler"
	"github.com/teapotovh/teapot/service/files/store"
)

const (
	archiveFileMode      = 0o644
	archiveDirectoryMode = 0o755
	archiveFallbackName  = "files"
)

func archiveName(name string) string {
	if name == "" {
		name = archiveFallbackName
	}

	return name + ".tar.gz"
}

func archivePathName(filePath string) string {
	name := path.Base(filePath)
	if name == "/" || name == "." {
		return archiveFallbackName
	}

	return name
}

func writeTarGz(out io.Writer, ctx context.Context, fs *store.FS, paths []string) (err error) {
	compressor := gzip.NewWriter(out)
	archive := tar.NewWriter(compressor)

	for _, filePath := range paths {
		if err := addToTarGz(ctx, archive, fs, filePath, archivePathName(filePath)); err != nil {
			err = errors.Join(err, archive.Close(), compressor.Close())
			return err
		}
	}

	if closeErr := archive.Close(); closeErr != nil {
		err = errors.Join(fmt.Errorf("could not close tar archive: %w", closeErr), err)
	}

	if closeErr := compressor.Close(); closeErr != nil {
		err = errors.Join(fmt.Errorf("could not close gzip archive: %w", closeErr), err)
	}

	return err
}

func addToTarGz(ctx context.Context, archive *tar.Writer, fs *store.FS, filePath, name string) error {
	inode, err := fs.Stat(ctx, filePath)
	if err != nil {
		return webhandler.NewInternalError(fmt.Errorf("could not stat %q: %w", filePath, err), nil)
	}

	if inode.Kind != store.KindDirectory {
		return addFileToTarGz(ctx, archive, fs, filePath, name, inode)
	}

	header := &tar.Header{
		Name:     path.Clean(name) + "/",
		Mode:     archiveDirectoryMode,
		Typeflag: tar.TypeDir,
		ModTime:  inode.ModTime,
	}

	if err := archive.WriteHeader(header); err != nil {
		return fmt.Errorf("could not write archive entry for %q: %w", name, err)
	}

	var cursor *store.Cursor

	for {
		page, err := fs.List(ctx, filePath, cursor)
		if err != nil {
			return webhandler.NewInternalError(fmt.Errorf("could not list directory at %q: %w", filePath, err), nil)
		}

		for _, child := range page.Inodes {
			childPath := path.Join(filePath, child.Name)
			childName := path.Join(name, child.Name)

			if err := addToTarGz(ctx, archive, fs, childPath, childName); err != nil {
				return err
			}
		}

		if page.Cursor == nil {
			break
		}

		cursor = page.Cursor
	}

	return nil
}

func addFileToTarGz(
	ctx context.Context,
	archive *tar.Writer,
	fs *store.FS,
	filePath, name string,
	inode store.Inode,
) (err error) {
	file, err := fs.Open(ctx, filePath)
	if err != nil {
		return webhandler.NewInternalError(fmt.Errorf("could not open file at %q: %w", filePath, err), nil)
	}

	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			err = errors.Join(fmt.Errorf("could not close file at %q: %w", filePath, closeErr), err)
		}
	}()

	header := &tar.Header{
		Name:     path.Clean(name),
		Mode:     archiveFileMode,
		Size:     inode.Size,
		Typeflag: tar.TypeReg,
		ModTime:  inode.ModTime,
	}

	if err := archive.WriteHeader(header); err != nil {
		return fmt.Errorf("could not write archive entry for %q: %w", name, err)
	}

	if _, err := io.Copy(archive, file); err != nil {
		return fmt.Errorf("could not write archive entry for %q: %w", name, err)
	}

	return nil
}
