package web

import (
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/teapotovh/teapot/lib/ui"
	"github.com/teapotovh/teapot/lib/webauth"
	"github.com/teapotovh/teapot/lib/webhandler"
)

const (
	browseSelectionFormID = "browse-selection"
	selectionPathID       = "path"

	selectionActionDownload = "download"
	selectionActionRename   = "rename"
	selectionActionDelete   = "delete"
)

func (web *Web) Rename(w http.ResponseWriter, r *http.Request) (ui.Component, error) {
	auth := webauth.GetAuth(r)
	if auth == nil {
		return nil, webhandler.NewRedirectError(PathIndex, http.StatusFound)
	}

	if r.Method != http.MethodPost {
		return nil, fmt.Errorf("invalid method %q: %w", r.Method, webhandler.ErrBadRequest)
	}

	rel, err := urlPath(PathRename, r.URL.Path)
	if err != nil {
		return nil, err
	}

	name, err := relativePath(r.FormValue(renameDialogNameID))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return dialogError{err: fmt.Errorf("invalid rename path: %w", err)}, nil
	}

	oldPath := storePath(rel)
	newPath := path.Join(path.Dir(oldPath), name)

	session, err := web.files.Session(r.Context(), auth.Username)
	if err != nil {
		return nil, webhandler.NewInternalError(err, nil)
	}

	if err := session.FS().Rename(r.Context(), oldPath, newPath); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return dialogError{err: fmt.Errorf("could not rename %q: %w", oldPath, err)}, nil
	}

	return nil, webhandler.NewRedirectError(PathBrowseDirAt(path.Dir(rel)), http.StatusFound)
}

func (web *Web) Delete(w http.ResponseWriter, r *http.Request) (ui.Component, error) {
	auth := webauth.GetAuth(r)
	if auth == nil {
		return nil, webhandler.NewRedirectError(PathIndex, http.StatusFound)
	}

	if r.Method != http.MethodPost {
		return nil, fmt.Errorf("invalid method %q: %w", r.Method, webhandler.ErrBadRequest)
	}

	rel, err := urlPath(PathDelete, r.URL.Path)
	if err != nil {
		return nil, err
	}

	paths, err := selectedPaths(r)
	if err != nil {
		return nil, err
	}

	session, err := web.files.Session(r.Context(), auth.Username)
	if err != nil {
		return nil, webhandler.NewInternalError(err, nil)
	}

	for _, selected := range paths {
		filePath := storePath(path.Join(rel, selected))
		if err := session.FS().Delete(r.Context(), filePath); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return dialogError{err: fmt.Errorf("could not delete %q: %w", filePath, err)}, nil
		}
	}

	return nil, webhandler.NewRedirectError(PathBrowseDirAt(rel), http.StatusFound)
}

func selectedPaths(r *http.Request) ([]string, error) {
	if err := r.ParseForm(); err != nil {
		return nil, errors.Join(fmt.Errorf("could not parse form: %w", err), webhandler.ErrBadRequest)
	}

	values := r.Form[selectionPathID]
	if len(values) == 0 {
		return nil, fmt.Errorf("no entries selected: %w", webhandler.ErrBadRequest)
	}

	paths := make([]string, 0, len(values))

	for _, value := range values {
		filePath, err := relativePath(value)
		if err != nil {
			return nil, fmt.Errorf("invalid selected path %q: %w", value, webhandler.ErrBadRequest)
		}

		paths = append(paths, filePath)
	}

	return paths, nil
}

func relativePath(value string) (string, error) {
	filePath := path.Clean(value)

	if filePath == "." || filePath == parentDir || path.IsAbs(filePath) || strings.HasPrefix(filePath, parentDir+"/") {
		return "", fmt.Errorf("invalid relative path %q: %w", value, webhandler.ErrBadRequest)
	}

	return filePath, nil
}
