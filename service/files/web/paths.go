package web

import (
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
