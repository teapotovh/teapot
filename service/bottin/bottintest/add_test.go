package bottintest

import (
	"testing"

	"github.com/go-ldap/ldap/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Adapted from:
// https://github.com/rumba-id/bauble/blob/ce079b46a8996ea0b8a1ec5bd07def580944579b/src/bauble/suites/rfc4511/add.py
//
//nolint:paralleltest
func TestBaubleAdd(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	carolDN := "uid=carol," + peopleDN

	t.Run("add", func(t *testing.T) {
		conn := server.dialRoot(t)

		require.NoError(t, conn.Add(newUserAddRequest(carolDN, "carol", "Carol Carter")))

		result := search(t, conn, carolDN, ldap.ScopeBaseObject, "(objectClass=*)", nil)
		require.Len(t, result.Entries, 1)
		assert.Equal(t, "carol", result.Entries[0].GetAttributeValue("uid"))
	})

	t.Run("duplicate", func(t *testing.T) {
		conn := server.dialRoot(t)

		requireLDAPError(
			t,
			conn.Add(newUserAddRequest(carolDN, "carol", "Carol Carter")),
			ldap.LDAPResultEntryAlreadyExists,
		)
	})

	t.Run("missing parent", func(t *testing.T) {
		conn := server.dialRoot(t)

		dn := "uid=missing,ou=missing," + baseDN
		requireLDAPError(t, conn.Add(newUserAddRequest(dn, "missing", "Missing Person")), ldap.LDAPResultNoSuchObject)
	})

	t.Run("restricted attribute", func(t *testing.T) {
		conn := server.dialRoot(t)

		dn := "uid=restricted," + peopleDN
		request := newUserAddRequest(dn, "restricted", "Restricted User")
		request.Attributes = append(request.Attributes, ldap.Attribute{
			Type: "memberOf",
			Vals: []string{"cn=admins," + groupsDN},
		})

		requireLDAPError(t, conn.Add(request), ldap.LDAPResultObjectClassViolation)
	})
}
