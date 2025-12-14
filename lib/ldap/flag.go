package ldap

import (
	flag "github.com/spf13/pflag"
)

func LDAPFlagSet() (*flag.FlagSet, func() LDAPConfig) {
	fs := flag.NewFlagSet("ldap", flag.ExitOnError)

	url := fs.String("ldap-url", "ldap://localhost:389", "the URI used to connect to LDAP")
	rootDN := fs.String("ldap-root-dn", "dc=teapot,dc=ovh", "the root DN to use for priviledged binds")
	rootPasswd := fs.String("ldap-root-passwd", "", "the passwd to bind to the root DN")
	usersDN := fs.String("ldap-users-dn", "ou=users,dc=teapot,dc=ovh", "the base DN where all users are stored")
	usersFilter := fs.String("ldap-users-filter", "(&(objectClass=inetOrgPerson)(cn={{ .Username }}))", "a templated filter to identify a unique user given the username")
	groupsDN := fs.String("ldap-groups-dn", "ou=groups,dc=teapot,dc=ovh", "the base DN where all groups are stored")
	adminGroupDN := fs.String("ldap-admin-group-dn", "cn=admin,ou=groups,dc=teapot,dc=ovh", "the DN of the group for admin users")
	accessesDN := fs.String("ldap-accesses-dn", "ou=accesses,dc=teapot,dc=ovh", "the base DN where access groups are stored")

	return fs, func() LDAPConfig {
		return LDAPConfig{
			URL:        *url,
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
