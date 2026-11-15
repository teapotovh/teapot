package bottintest

import (
	"testing"

	"github.com/go-ldap/ldap/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Local test. Bauble does not assert memberOf maintenance. The group schema is
// from RFC 4519 section 3.6:
// https://www.rfc-editor.org/rfc/rfc4519#section-3.6
func TestBaubleGroupMembership(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	groupDN := "cn=testgroup," + groupsDN

	conn := server.dialRoot(t)
	require.NoError(t, conn.Add(groupAddRequest(groupDN, "testgroup", aliceDN)))

	result := search(t, conn, aliceDN, ldap.ScopeBaseObject, "(objectClass=*)", []string{"memberOf"})
	require.Len(t, result.Entries, 1)
	assert.Contains(t, result.Entries[0].GetEqualFoldAttributeValues("memberOf"), groupDN)

	request := ldap.NewModifyRequest(groupDN, nil)
	request.Add("member", []string{bobDN})
	require.NoError(t, conn.Modify(request))

	result = search(t, conn, bobDN, ldap.ScopeBaseObject, "(objectClass=*)", []string{"memberOf"})
	require.Len(t, result.Entries, 1)
	assert.Contains(t, result.Entries[0].GetEqualFoldAttributeValues("memberOf"), groupDN)

	require.NoError(t, conn.Del(ldap.NewDelRequest(groupDN, nil)))

	result = search(t, conn, bobDN, ldap.ScopeBaseObject, "(objectClass=*)", []string{"memberOf"})
	require.Len(t, result.Entries, 1)
	assert.NotContains(t, result.Entries[0].GetEqualFoldAttributeValues("memberOf"), groupDN)
}
