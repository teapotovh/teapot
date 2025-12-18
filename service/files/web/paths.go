package web

import "path"

const (
	App      = "Teapot Files"
	AppShort = "Files"

	PathLogin  = "/login"
	PathLogout = "/logout"

	PathIndex  = "/"
	PathBrowse = "/browse/"
)

func PathBrowseAt(subpath string) string {
	return path.Join(PathBrowse, subpath)
}
