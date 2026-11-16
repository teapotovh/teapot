package bottintest

import (
	"context"
	"strings"
	"testing"

	"github.com/go-ldap/ldap/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/teapotovh/teapot/service/bottin/store"
)

// Adapted from:
// https://github.com/rumba-id/bauble/blob/ce079b46a8996ea0b8a1ec5bd07def580944579b/src/bauble/suites/rfc4511/modify.py
//
//nolint:paralleltest
func TestBaubleModify(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	carolDN := "uid=carol," + peopleDN

	conn := server.dialRoot(t)
	require.NoError(t, conn.Add(newUserAddRequest(carolDN, "carol", "Carol Carter")))

	t.Run("replace", func(t *testing.T) {
		conn := server.dialRoot(t)

		request := ldap.NewModifyRequest(carolDN, nil)
		request.Replace("cn", []string{"Carol Cooper"})
		require.NoError(t, conn.Modify(request))

		result := search(t, conn, carolDN, ldap.ScopeBaseObject, "(objectClass=*)", nil)
		require.Len(t, result.Entries, 1)
		assert.Equal(t, "Carol Cooper", result.Entries[0].GetAttributeValue("cn"))
	})

	t.Run("add and delete values", func(t *testing.T) {
		conn := server.dialRoot(t)

		request := ldap.NewModifyRequest(carolDN, nil)
		request.Add("mail", []string{"carol@example.com"})
		require.NoError(t, conn.Modify(request))

		result := search(t, conn, carolDN, ldap.ScopeBaseObject, "(objectClass=*)", nil)
		require.Len(t, result.Entries, 1)
		assert.Equal(t, "carol@example.com", result.Entries[0].GetAttributeValue("mail"))

		request = ldap.NewModifyRequest(carolDN, nil)
		request.Delete("mail", []string{"carol@example.com"})
		require.NoError(t, conn.Modify(request))

		result = search(t, conn, carolDN, ldap.ScopeBaseObject, "(objectClass=*)", nil)
		require.Len(t, result.Entries, 1)
		assert.Empty(t, result.Entries[0].GetAttributeValue("mail"))
	})

	t.Run("restricted attribute", func(t *testing.T) {
		conn := server.dialRoot(t)

		request := ldap.NewModifyRequest(carolDN, nil)
		request.Replace("memberOf", []string{"cn=admins," + groupsDN})
		requireLDAPError(t, conn.Modify(request), ldap.LDAPResultObjectClassViolation)
	})

	t.Run("missing entry", func(t *testing.T) {
		conn := server.dialRoot(t)

		request := ldap.NewModifyRequest("uid=missing,"+peopleDN, nil)
		request.Replace("cn", []string{"Missing"})
		requireLDAPError(t, conn.Modify(request), ldap.LDAPResultNoSuchObject)
	})
}

// Local test. Member values are DNs and must match without case.
func TestModifyMemberCase(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	groupDN := "cn=mixed-member," + groupsDN

	dn, err := store.ParseDN(groupDN)
	require.NoError(t, err)

	ctx := context.Background()

	tx, err := server.bottin.Store().Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, tx.Store(ctx, store.NewEntry(dn, store.Attributes{
		store.NewAttributeKey("objectclass"): store.AttributeValue{objectClassTop, "groupOfNames"},
		store.NewAttributeKey("member"):      store.AttributeValue{strings.ToUpper(aliceDN)},
	})))
	require.NoError(t, tx.Commit(ctx))

	conn := server.dialRoot(t)

	add := ldap.NewModifyRequest(groupDN, nil)
	add.Add("member", []string{aliceDN})
	require.NoError(t, conn.Modify(add))

	result := search(t, conn, groupDN, ldap.ScopeBaseObject, "(objectClass=*)", []string{"member"})
	require.Len(t, result.Entries, 1)
	assert.Len(t, result.Entries[0].GetEqualFoldAttributeValues("member"), 1)

	del := ldap.NewModifyRequest(groupDN, nil)
	del.Delete("member", []string{aliceDN})
	require.NoError(t, conn.Modify(del))

	result = search(t, conn, groupDN, ldap.ScopeBaseObject, "(objectClass=*)", []string{"member"})
	require.Len(t, result.Entries, 1)
	assert.Empty(t, result.Entries[0].GetEqualFoldAttributeValues("member"))
}
