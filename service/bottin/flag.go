package bottin

import (
	flag "github.com/spf13/pflag"
	"github.com/teapotovh/teapot/service/bottin/store"
)

var (
	DefaultBaseDN = "dc=teapot,dc=ovh"
	DefaultACL    = []string{
		"ANONYMOUS::bind:*,ou=users,dc=teapot,dc=ovh:",
		"ANONYMOUS::bind:dc=teapot,dc=ovh:",
		"*,dc=teapot,dc=ovh::read:*:* !userpassword",
		"*::read modify:SELF:*",
		"dc=teapot,dc=ovh::read add modify delete:*:*",
		"*:cn=admin,ou=groups,dc=teapot,dc=ovh:read add modify delete:*:*",
	}
)

func BottinFlagSet() (*flag.FlagSet, func() BottinConfig) {
	fs := flag.NewFlagSet("log", flag.ExitOnError)

	baseDN := fs.String("bottin-basedn", DefaultBaseDN, "the base DN of the LDAP server")
	passwd := fs.String("bottin-passwd", "", "the passwd for binding to the root object")
	acl := fs.StringArray("bottin-acl", DefaultACL, "the list of ACL rules to apply for permission checking")

	certFile := fs.String("bottin-tls-cert", "", "the path to the TLS certificate file")
	keyFile := fs.String("bottin-key-file", "", "the path to the TLS key file")
	serverName := fs.String("bottin-server-name", "", "the DNS name of the LDAP server for TLS")

	storeFS, getStoreConfig := store.StoreFlagSet()
	fs.AddFlagSet(storeFS)

	return fs, func() BottinConfig {
		return BottinConfig{
			BaseDN: *baseDN,
			Passwd: *passwd,
			ACL:    *acl,

			TLSCertFile:   *certFile,
			TLSKeyFile:    *keyFile,
			TLSServerName: *serverName,

			Store: getStoreConfig(),
		}
	}
}
