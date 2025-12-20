package web

import (
	"path/filepath"
)

const (
	App      = "Teapot Files"
	AppShort = "Files"

	PathLogin  = "/login"
	PathLogout = "/logout"

	PathIndex  = "/"
	PathBrowse = "/browse/"
	PathFile   = "/file/"
)

func PathBrowseAt(paths ...string) string {
	return filepath.Join(append([]string{PathBrowse}, paths...)...)
}

func PathFileAt(paths ...string) string {
	return filepath.Join(append([]string{PathFile}, paths...)...)
}
