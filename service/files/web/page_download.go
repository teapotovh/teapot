package web

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path"

	"github.com/arhuman/bizkit/fileserve"

	"github.com/teapotovh/teapot/lib/httphandler"
	"github.com/teapotovh/teapot/lib/webauth"
	"github.com/teapotovh/teapot/service/files/store"
)

const browseArchiveName = "files"

func (web *Web) Download(w http.ResponseWriter, r *http.Request) error {
	auth := webauth.GetAuth(r)
	if auth == nil {
		return httphandler.NewRedirectError(PathIndex, http.StatusFound)
	}

	rel, err := urlPath(PathDownload, r.URL.Path)
	if err != nil {
		return fmt.Errorf("could not get relative path: %w", err)
	}

	session, err := web.files.Session(r.Context(), auth.Username)
	if err != nil {
		return httphandler.NewInternalError(err, nil)
	}

	fs := session.FS()

	switch r.Method {
	case http.MethodGet:
		return downloadPath(w, r, fs, storePath(rel))

	case http.MethodPost:
		paths, err := selectedPaths(r)
		if err != nil {
			return err
		}

		if len(paths) == 1 {
			filePath := storePath(path.Join(rel, paths[0]))
			return httphandler.NewRedirectError(EscapePath(PathDownloadAt(filePath)), http.StatusFound)
		}

		entries := make([]string, 0, len(paths))
		for _, selected := range paths {
			entries = append(entries, storePath(path.Join(rel, selected)))
		}

		w.Header().Set("Content-Type", "application/gzip")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{
			"filename": archiveName(browseArchiveName),
		}))

		return writeTarGz(w, r.Context(), fs, entries)

	default:
		return fmt.Errorf("invalid method %q: %w", r.Method, httphandler.ErrBadRequest)
	}
}

func downloadPath(w http.ResponseWriter, r *http.Request, fs *store.FS, filePath string) error {
	inode, err := fs.Stat(r.Context(), filePath)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return httphandler.ErrNotFound
		}

		return httphandler.NewInternalError(fmt.Errorf("could not stat %q: %w", filePath, err), nil)
	}

	if inode.Kind == store.KindDirectory {
		w.Header().Set("Content-Type", "application/gzip")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{
			"filename": archiveName(inode.Name),
		}))

		return writeTarGz(w, r.Context(), fs, []string{filePath})
	}

	return serveFile(w, r, fs, filePath, inode)
}

func serveFile(w http.ResponseWriter, r *http.Request, fs *store.FS, filePath string, inode store.Inode) (err error) {
	file, err := fs.Open(r.Context(), filePath)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) || errors.Is(err, store.ErrIsDirectory) {
			return httphandler.ErrNotFound
		}

		return httphandler.NewInternalError(fmt.Errorf("could not read file at %q: %w", filePath, err), nil)
	}

	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			err = errors.Join(fmt.Errorf("could not close file at %q: %w", filePath, closeErr), err)
		}
	}()

	contentType, err := detectContentType(file)
	if err != nil {
		return httphandler.NewInternalError(fmt.Errorf("could not detect content type for %q: %w", filePath, err), nil)
	}

	fileserve.Serve(w, r, fileserve.Meta{
		Name:        inode.Name,
		ContentType: contentType,
		ModTime:     inode.ModTime,
	}, file)

	return nil
}

// detectContentType returns the media type of the file.
func detectContentType(file store.File) (string, error) {
	contentType, _, err := fileserve.Sniff(file)
	if err != nil {
		return "", fmt.Errorf("could not detect content type: %w", err)
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("could not seek file: %w", err)
	}

	return contentType, nil
}
