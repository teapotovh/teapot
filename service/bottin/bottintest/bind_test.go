package bottintest

import (
	"testing"

	"github.com/go-ldap/ldap/v3"
	"github.com/stretchr/testify/require"
)

// Adapted from:
// https://github.com/rumba-id/bauble/blob/ce079b46a8996ea0b8a1ec5bd07def580944579b/src/bauble/suites/rfc4511/bind.py
func TestBaubleBind(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	t.Run("anonymous", func(t *testing.T) {
		t.Parallel()
		t.Skip(skipAnonymousBind)
	})

	t.Run("root", func(t *testing.T) {
		t.Parallel()

		conn := server.dial(t)
		require.NoError(t, conn.Bind(baseDN, rootPasswd))
	})

	t.Run("simple", func(t *testing.T) {
		t.Parallel()

		conn := server.dial(t)
		require.NoError(t, conn.Bind(aliceDN, alicePasswd))
	})

	t.Run("invalid password", func(t *testing.T) {
		t.Parallel()

		conn := server.dial(t)
		requireLDAPError(t, conn.Bind(aliceDN, "wrong-password"), ldap.LDAPResultInvalidCredentials)
	})

	t.Run("empty password", func(t *testing.T) {
		t.Parallel()

		conn := server.dial(t)
		requireLDAPError(t, conn.UnauthenticatedBind(aliceDN), ldap.LDAPResultInvalidCredentials)
	})

	t.Run("rebind", func(t *testing.T) {
		t.Parallel()

		conn := server.dial(t)
		require.NoError(t, conn.Bind(aliceDN, alicePasswd))
		require.NoError(t, conn.Bind(bobDN, bobPasswd))
	})

	t.Run("bad protocol version", func(t *testing.T) {
		t.Parallel()
		t.Skip(skipBadVersion)
	})

	t.Run("sasl", func(t *testing.T) {
		t.Parallel()
		t.Skip(skipSASLBind)
	})
}
