package files

import (
	flag "github.com/spf13/pflag"

	"github.com/teapotovh/teapot/lib/ldap"
	"github.com/teapotovh/teapot/service/files/store"
)

// FilesFlagSet returns the flags for a Files service config.
func FilesFlagSet() (*flag.FlagSet, func() FilesConfig) {
	fs := flag.NewFlagSet("files", flag.ExitOnError)

	storeFS, getStoreConfig := store.StoreFlagSet()
	fs.AddFlagSet(storeFS)

	ldapFS, getLDAPConfig := ldap.LDAPFlagSet()
	fs.AddFlagSet(ldapFS)

	return fs, func() FilesConfig {
		return FilesConfig{
			Store: getStoreConfig(),
			LDAP:  getLDAPConfig(),
		}
	}
}
