package web

import (
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/teapotovh/teapot/lib/webhandler"
)

const (
	App      = "Teapot Files"
	AppShort = "Files"

	PathLogin  = "/login"
	PathLogout = "/logout"

	PathIndex         = "/"
	PathBrowse        = "/browse/"
	PathBrowseDialog  = "/internal/browse/dialog/"
	PathBrowseEntries = "/internal/browse/entries/"
	PathFile          = "/file/"
)

func PathBrowseAt(paths ...string) string {
	return path.Join(append([]string{PathBrowse}, paths...)...)
}

// PathBrowseDirAt returns the browse URL of a directory with a trailing slash.
func PathBrowseDirAt(paths ...string) string {
	return PathBrowseAt(paths...) + "/"
}

func PathBrowseDialogOf(dialog string) string {
	return path.Join(PathBrowseDialog, dialog)
}

func PathFileAt(paths ...string) string {
	return path.Join(append([]string{PathFile}, paths...)...)
}

// EscapePath percent-encodes a path for use in a URL.
func EscapePath(path string) string {
	return (&url.URL{Path: path}).EscapedPath()
}

// storePath returns the absolute store path of a relative path.
func storePath(rel string) string {
	return path.Join("/", rel)
}

// urlPath returns the cleaned path of value relative to prefix.
func urlPath(prefix, value string) (string, error) {
	if !strings.HasPrefix(value, prefix) {
		return "", fmt.Errorf("path %q is not under prefix %q: %w", value, prefix, webhandler.ErrBadRequest)
	}

	return path.Clean(strings.TrimPrefix(value, prefix)), nil
}
