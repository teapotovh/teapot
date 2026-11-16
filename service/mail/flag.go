package mail

import (
	flag "github.com/spf13/pflag"

	"github.com/teapotovh/teapot/lib/ldap"
	"github.com/teapotovh/teapot/service/mail/store"
)

func MailFlagSet() (*flag.FlagSet, func() Config) {
	fs := flag.NewFlagSet("mail", flag.ExitOnError)

	storeFS, getStoreConfig := store.StoreFlagSet()
	fs.AddFlagSet(storeFS)

	ldapFS, getLDAPConfig := ldap.LDAPFlagSet()
	fs.AddFlagSet(ldapFS)

	return fs, func() Config {
		return Config{
			Store: getStoreConfig(),
			LDAP:  getLDAPConfig(),
		}
	}
}
