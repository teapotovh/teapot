package files

import (
	flag "github.com/spf13/pflag"

	"github.com/teapotovh/teapot/lib/ldap"
)

func FilesFlagSet() (*flag.FlagSet, func() FilesConfig) {
	fs := flag.NewFlagSet("files", flag.ExitOnError)

	mounts := fs.StringSliceP("files-mount", "m", nil, "declare a list of mount points")
	sessionsFS, getSessionsConfig := SessionsFlagSet()
	fs.AddFlagSet(sessionsFS)

	ldapFS, getLdapConfig := ldap.LDAPFlagSet()
	fs.AddFlagSet(ldapFS)

	return fs, func() FilesConfig {
		sessions := getSessionsConfig()
		sessions.Mounts = *mounts

		return FilesConfig{
			Mounts:   *mounts,
			Sessions: sessions,
			LDAP:     getLdapConfig(),
		}
	}
}
