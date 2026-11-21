package web

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"

	"github.com/teapotovh/teapot/lib/httphandler"
	"github.com/teapotovh/teapot/lib/webauth"
	"github.com/teapotovh/teapot/service/files/store"
)

func (web *Web) File(w http.ResponseWriter, r *http.Request) (err error) {
	auth := webauth.GetAuth(r)
	if auth == nil {
		return httphandler.NewRedirectError(PathIndex, http.StatusFound)
	}

	path, err := filepath.Rel(PathFile, r.URL.Path)
	if err != nil {
		return errors.Join(fmt.Errorf("could not get relative path: %w", err), httphandler.ErrBadRequest)
	}

	path = filepath.Clean(path)

	session, err := web.files.Session(r.Context(), auth.Username)
	if err != nil {
		return httphandler.NewInternalError(err, nil)
	}

	file, err := session.FS().Open(r.Context(), sep+path)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return httphandler.ErrNotFound
		}

		return httphandler.NewInternalError(fmt.Errorf("could not read file at %q: %w", path, err), nil)
	}

	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			err = errors.Join(fmt.Errorf("could not close file at %q: %w", path, closeErr), err)
		}
	}()

	if _, err := io.Copy(w, file); err != nil {
		return fmt.Errorf("error while writing response file at %q: %w", path, err)
	}

	return nil
}
