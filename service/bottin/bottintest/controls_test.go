package bottintest

import (
	"testing"

	"github.com/go-ldap/ldap/v3"
	"github.com/stretchr/testify/require"
)

// Adapted from:
// https://github.com/rumba-id/bauble/blob/ce079b46a8996ea0b8a1ec5bd07def580944579b/src/bauble/suites/rfc4511/controls.py
func TestRequestControls(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	t.Run("unknown critical", func(t *testing.T) {
		t.Parallel()

		conn := server.dialRoot(t)

		_, err := conn.Search(ldap.NewSearchRequest(
			baseDN,
			ldap.ScopeBaseObject,
			ldap.NeverDerefAliases,
			0,
			0,
			false,
			"(objectClass=*)",
			nil,
			[]ldap.Control{ldap.NewControlString("1.2.3.4.5-fake", true, "")},
		))
		requireLDAPError(t, err, ldap.LDAPResultUnavailableCriticalExtension)
	})

	t.Run("unknown non-critical", func(t *testing.T) {
		t.Parallel()

		conn := server.dialRoot(t)

		_, err := conn.Search(ldap.NewSearchRequest(
			baseDN,
			ldap.ScopeBaseObject,
			ldap.NeverDerefAliases,
			0,
			0,
			false,
			"(objectClass=*)",
			nil,
			[]ldap.Control{ldap.NewControlString("1.2.3.4.5-fake", false, "")},
		))
		require.NoError(t, err)
	})
}
