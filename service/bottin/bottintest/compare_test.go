package bottintest

import (
	"testing"

	"github.com/go-ldap/ldap/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Adapted from:
// https://github.com/rumba-id/bauble/blob/ce079b46a8996ea0b8a1ec5bd07def580944579b/src/bauble/suites/rfc4511/compare.py
func TestBaubleCompare(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	t.Run("true", func(t *testing.T) {
		t.Parallel()

		conn := server.dialRoot(t)

		matched, err := conn.Compare(aliceDN, "uid", "alice")
		require.NoError(t, err)
		assert.True(t, matched)
	})

	t.Run("false", func(t *testing.T) {
		t.Parallel()

		conn := server.dialRoot(t)

		matched, err := conn.Compare(aliceDN, "uid", "bob")
		require.NoError(t, err)
		assert.False(t, matched)
	})

	t.Run("missing entry", func(t *testing.T) {
		t.Parallel()

		conn := server.dialRoot(t)

		_, err := conn.Compare("uid=missing,"+peopleDN, "uid", "missing")
		requireLDAPError(t, err, ldap.LDAPResultNoSuchObject)
	})
}
