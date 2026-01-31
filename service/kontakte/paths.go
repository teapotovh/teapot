package kontakte

import (
	"path/filepath"
)

const (
	App        = "Teapot Kontakte"
	AppShort   = "Kontakte"
	PageUsers  = "Users"
	PageGroups = "Groups"

	PathLogin  = "/login"
	PathLogout = "/logout"

	PathIndex  = "/"
	PathUsers  = "/users"
	PathGroups = "/groups"
)

func PathUser(username string) string {
	return filepath.Join(PathUsers, username)
}

func PathPasswd(username string) string {
	return filepath.Join(PathUsers, username, "passwd")
}

func PathGroup(name string) string {
	return filepath.Join(PathGroups, name)
}
