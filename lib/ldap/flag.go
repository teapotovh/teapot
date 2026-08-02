package ldap

import (
	"time"

	flag "github.com/spf13/pflag"
)

func LDAPFlagSet() (*flag.FlagSet, func() LDAPConfig) {
	fs := flag.NewFlagSet("ldap", flag.ExitOnError)

	url := fs.String("ldap-url", "ldap://localhost:389", "the URI used to connect to LDAP")
	timeout := fs.Duration("ldap-timeout", 2*time.Second, "the LDAP connection timeout")
	maxConnections := fs.Uint32(
		"ldap-max-connections",
		4,
		"maximum number of LDAP connections to keep alive for pooling",
	)
	workers := fs.Uint32(
		"ldap-pool-workers",
		2,
		"number of workers that constantly fill the pool with LDAP connections",
	)
	rootDN := fs.String("ldap-root-dn", "dc=teapot,dc=ovh", "the root DN to use for priviledged binds")
	rootPasswd := fs.String("ldap-root-passwd", "", "the passwd to bind to the root DN")
	usersDN := fs.String("ldap-users-dn", "ou=users,dc=teapot,dc=ovh", "the base DN where all users are stored")
	usersFilter := fs.String(
		"ldap-users-filter",
		"(&(objectClass=inetOrgPerson)(cn={{ .Username }}))",
		"a templated filter to identify a unique user given the username",
	)
	groupsDN := fs.String("ldap-groups-dn", "ou=groups,dc=teapot,dc=ovh", "the base DN where all groups are stored")
	adminGroupDN := fs.String(
		"ldap-admin-group-dn",
		"cn=admin,ou=groups,dc=teapot,dc=ovh",
		"the DN of the group for admin users",
	)
	accessesDN := fs.String(
		"ldap-accesses-dn",
		"ou=accesses,dc=teapot,dc=ovh",
		"the base DN where access groups are stored",
	)

	return fs, func() LDAPConfig {
		return LDAPConfig{
			URL:            *url,
			Timeout:        *timeout,
			MaxConnections: *maxConnections,
			Workers:        *workers,

			RootDN:     *rootDN,
			RootPasswd: *rootPasswd,

			UsersDN:      *usersDN,
			UsersFilter:  *usersFilter,
			GroupsDN:     *groupsDN,
			AdminGroupDN: *adminGroupDN,
			AccessesDN:   *accessesDN,
		}
	}
}
