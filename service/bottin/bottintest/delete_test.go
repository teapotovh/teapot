package bottintest

import (
	"testing"

	"github.com/go-ldap/ldap/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Adapted from:
// https://github.com/rumba-id/bauble/blob/ce079b46a8996ea0b8a1ec5bd07def580944579b/src/bauble/suites/rfc4511/delete.py
//
//nolint:paralleltest
func TestBaubleDelete(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	carolDN := "uid=carol," + peopleDN

	conn := server.dialRoot(t)
	require.NoError(t, conn.Add(newUserAddRequest(carolDN, "carol", "Carol Carter")))

	t.Run("delete", func(t *testing.T) {
		conn := server.dialRoot(t)

		require.NoError(t, conn.Del(ldap.NewDelRequest(carolDN, nil)))

		result := search(t, conn, peopleDN, ldap.ScopeSingleLevel, "(uid=carol)", nil)
		assert.Empty(t, result.Entries)
	})

	t.Run("non leaf", func(t *testing.T) {
		conn := server.dialRoot(t)

		requireLDAPError(t, conn.Del(ldap.NewDelRequest(peopleDN, nil)), ldap.LDAPResultNotAllowedOnNonLeaf)
	})
}
