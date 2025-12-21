package web

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/hack-pad/hackpadfs"

	"github.com/teapotovh/teapot/lib/httphandler"
	"github.com/teapotovh/teapot/lib/webauth"
)

func (web *Web) File(w http.ResponseWriter, r *http.Request) error {
	auth := webauth.GetAuth(r)
	if auth == nil {
		return httphandler.NewRedirectError(PathIndex, http.StatusFound)
	}

	path, err := filepath.Rel(PathFile, r.URL.Path)
	if err != nil {
		return errors.Join(fmt.Errorf("could not get relative path: %w", err), httphandler.ErrBadRequest)
	}

	path = filepath.Clean(path)

	session, err := web.files.Sesssions().Get(auth.Username)
	if err != nil {
		return httphandler.NewInternalError(err, nil)
	}

	file, err := hackpadfs.OpenFile(session.FS(), path, os.O_RDONLY, 0)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return httphandler.ErrNotFound
		}

		return httphandler.NewInternalError(fmt.Errorf("could not read file at %q: %w", path, err), nil)
	}

	_, err = io.Copy(w, file)
	if err != nil {
		return fmt.Errorf("error while writing response file at %q: %w", path, err)
	}

	return nil
}
