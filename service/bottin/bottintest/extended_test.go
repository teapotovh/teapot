package bottintest

import (
	"testing"

	"github.com/go-ldap/ldap/v3"
	"github.com/stretchr/testify/require"
)

// Adapted from:
// https://github.com/rumba-id/bauble/blob/ce079b46a8996ea0b8a1ec5bd07def580944579b/src/bauble/suites/extended/rfc3062.py
func TestBaublePasswordModify(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	const newPassword = "alice-new-secret"

	conn := server.dialAlice(t)
	_, err := conn.PasswordModify(ldap.NewPasswordModifyRequest("", "", newPassword))
	require.NoError(t, err)

	verify := server.dial(t)
	require.NoError(t, verify.Bind(aliceDN, newPassword))

	old := server.dial(t)
	requireLDAPError(t, old.Bind(aliceDN, alicePasswd), ldap.LDAPResultInvalidCredentials)

	t.Run("old password", func(t *testing.T) {
		t.Parallel()
		t.Skip(skipOldPassword)
	})
}
