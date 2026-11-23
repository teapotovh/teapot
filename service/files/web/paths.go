package web

import (
	"net/url"
	"path/filepath"
)

const (
	App      = "Teapot Files"
	AppShort = "Files"

	PathLogin  = "/login"
	PathLogout = "/logout"

	PathIndex        = "/"
	PathBrowse       = "/browse/"
	PathBrowseDialog = "/internal/browse/dialog/"
	PathFile         = "/file/"
)

func PathBrowseAt(paths ...string) string {
	return filepath.Join(append([]string{PathBrowse}, paths...)...)
}

func PathBrowseDialogOf(dialog string) string {
	return filepath.Join(PathBrowseDialog, dialog)
}

func PathFileAt(paths ...string) string {
	return filepath.Join(append([]string{PathFile}, paths...)...)
}

// EscapePath percent-encodes a path for use in a URL.
func EscapePath(path string) string {
	return (&url.URL{Path: path}).EscapedPath()
}
