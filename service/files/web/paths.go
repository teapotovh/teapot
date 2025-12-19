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
)

func PathBrowseAt(paths ...string) string {
	return filepath.Join(append([]string{PathBrowse}, paths...)...)
}
