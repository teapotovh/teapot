package bottintest

import (
	"testing"

	"github.com/go-ldap/ldap/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Adapted from:
// https://github.com/rumba-id/bauble/blob/ce079b46a8996ea0b8a1ec5bd07def580944579b/src/bauble/suites/rfc4511/moddn.py
//
//nolint:paralleltest
func TestBaubleModifyDN(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	t.Run("rename", func(t *testing.T) {
		conn := server.dialRoot(t)

		dn := "uid=carol-moddn," + peopleDN
		newDN := "uid=carol-moddn-renamed," + peopleDN

		require.NoError(t, conn.Add(newUserAddRequest(dn, "carol-moddn", "Carol Carter")))
		require.NoError(t, conn.ModifyDN(ldap.NewModifyDNRequest(dn, "uid=carol-moddn-renamed", true, "")))

		result := search(t, conn, newDN, ldap.ScopeBaseObject, "(objectClass=*)", nil)
		require.Len(t, result.Entries, 1)
		assert.Equal(t, []string{"carol-moddn-renamed"}, result.Entries[0].GetEqualFoldAttributeValues("uid"))
	})

	t.Run("rename to existing", func(t *testing.T) {
		conn := server.dialRoot(t)

		target := "uid=target-moddn," + peopleDN
		source := "uid=source-moddn," + peopleDN

		require.NoError(t, conn.Add(newUserAddRequest(target, "target-moddn", "Target User")))
		require.NoError(t, conn.Add(newUserAddRequest(source, "source-moddn", "Source User")))

		err := conn.ModifyDN(ldap.NewModifyDNRequest(source, "uid=target-moddn", true, ""))
		requireLDAPError(t, err, ldap.LDAPResultEntryAlreadyExists)
	})

	t.Run("move with new superior", func(t *testing.T) {
		conn := server.dialRoot(t)

		dn := "uid=move-moddn," + peopleDN
		newDN := "uid=move-moddn," + baseDN

		require.NoError(t, conn.Add(newUserAddRequest(dn, "move-moddn", "Moving User")))
		require.NoError(t, conn.ModifyDN(ldap.NewModifyDNRequest(dn, "uid=move-moddn", false, baseDN)))

		result := search(t, conn, newDN, ldap.ScopeBaseObject, "(objectClass=*)", nil)
		require.Len(t, result.Entries, 1)
	})

	t.Run("keep old rdn", func(t *testing.T) {
		conn := server.dialRoot(t)

		dn := "uid=keep-moddn," + peopleDN
		newDN := "uid=keep-moddn-renamed," + peopleDN

		require.NoError(t, conn.Add(newUserAddRequest(dn, "keep-moddn", "Keeping User")))
		require.NoError(t, conn.ModifyDN(ldap.NewModifyDNRequest(dn, "uid=keep-moddn-renamed", false, "")))

		result := search(t, conn, newDN, ldap.ScopeBaseObject, "(objectClass=*)", nil)
		require.Len(t, result.Entries, 1)
		assert.ElementsMatch(
			t,
			[]string{"keep-moddn", "keep-moddn-renamed"},
			result.Entries[0].GetEqualFoldAttributeValues("uid"),
		)
	})

	t.Run("member reference", func(t *testing.T) {
		conn := server.dialRoot(t)

		groupDN := "cn=renamers," + groupsDN
		dn := "uid=member-moddn," + peopleDN
		newDN := "uid=member-moddn-renamed," + peopleDN

		require.NoError(t, conn.Add(newUserAddRequest(dn, "member-moddn", "Member User")))
		require.NoError(t, conn.Add(groupAddRequest(groupDN, "renamers", dn)))
		require.NoError(t, conn.ModifyDN(ldap.NewModifyDNRequest(dn, "uid=member-moddn-renamed", true, "")))

		result := search(t, conn, groupDN, ldap.ScopeBaseObject, "(objectClass=*)", []string{"member"})
		require.Len(t, result.Entries, 1)
		assert.Equal(t, newDN, result.Entries[0].GetAttributeValue("member"))
	})

	t.Run("non leaf", func(t *testing.T) {
		conn := server.dialRoot(t)

		err := conn.ModifyDN(ldap.NewModifyDNRequest(peopleDN, "ou=people-renamed", true, ""))
		requireLDAPError(t, err, ldap.LDAPResultNotAllowedOnNonLeaf)
	})

	t.Run("group memberOf reference", func(t *testing.T) {
		conn := server.dialRoot(t)

		userDN := "uid=group-member-moddn," + peopleDN
		groupDN := "cn=old-group-moddn," + groupsDN
		newGroupDN := "cn=new-group-moddn," + groupsDN

		require.NoError(t, conn.Add(newUserAddRequest(userDN, "group-member-moddn", "Group Member")))
		require.NoError(t, conn.Add(groupAddRequest(groupDN, "old-group-moddn", userDN)))
		require.NoError(t, conn.ModifyDN(ldap.NewModifyDNRequest(groupDN, "cn=new-group-moddn", true, "")))

		result := search(t, conn, userDN, ldap.ScopeBaseObject, "(objectClass=*)", []string{"memberOf"})
		require.Len(t, result.Entries, 1)
		assert.Equal(t, newGroupDN, result.Entries[0].GetEqualFoldAttributeValue("memberOf"))
	})

	t.Run("move member reference", func(t *testing.T) {
		conn := server.dialRoot(t)

		groupDN := "cn=move-group-moddn," + groupsDN
		dn := "uid=move-member-moddn," + peopleDN
		newDN := "uid=move-member-moddn," + baseDN

		require.NoError(t, conn.Add(newUserAddRequest(dn, "move-member-moddn", "Moving Member")))
		require.NoError(t, conn.Add(groupAddRequest(groupDN, "move-group-moddn", dn)))
		require.NoError(t, conn.ModifyDN(ldap.NewModifyDNRequest(dn, "uid=move-member-moddn", false, baseDN)))

		result := search(t, conn, groupDN, ldap.ScopeBaseObject, "(objectClass=*)", []string{"member"})
		require.Len(t, result.Entries, 1)
		assert.Equal(t, newDN, result.Entries[0].GetAttributeValue("member"))
	})

	t.Run("missing parent", func(t *testing.T) {
		conn := server.dialRoot(t)

		dn := "uid=missing-parent-moddn," + peopleDN

		require.NoError(t, conn.Add(newUserAddRequest(dn, "missing-parent-moddn", "Missing Parent")))
		err := conn.ModifyDN(ldap.NewModifyDNRequest(dn, "uid=missing-parent-moddn", true, "ou=missing,"+baseDN))
		requireLDAPError(t, err, ldap.LDAPResultNoSuchObject)
	})

	t.Run("invalid new rdn", func(t *testing.T) {
		conn := server.dialRoot(t)

		dn := "uid=invalid-rdn-moddn," + peopleDN

		require.NoError(t, conn.Add(newUserAddRequest(dn, "invalid-rdn-moddn", "Invalid RDN")))
		err := conn.ModifyDN(ldap.NewModifyDNRequest(dn, "uid=one,uid=two", true, ""))
		requireLDAPError(t, err, ldap.LDAPResultInvalidDNSyntax)
	})

	t.Run("base entry", func(t *testing.T) {
		conn := server.dialRoot(t)

		err := conn.ModifyDN(ldap.NewModifyDNRequest(baseDN, "dc=other", true, ""))
		requireLDAPError(t, err, ldap.LDAPResultUnwillingToPerform)
	})

	t.Run("case insensitive rdn value", func(t *testing.T) {
		conn := server.dialRoot(t)

		dn := "uid=mixed-case-moddn," + peopleDN
		newDN := "cn=mixed case moddn," + peopleDN

		require.NoError(t, conn.Add(newUserAddRequest(dn, "mixed-case-moddn", "Mixed Case Moddn")))
		require.NoError(t, conn.ModifyDN(ldap.NewModifyDNRequest(dn, "cn=Mixed Case Moddn", true, "")))

		result := search(t, conn, newDN, ldap.ScopeBaseObject, "(objectClass=*)", nil)
		require.Len(t, result.Entries, 1)
		assert.Equal(t, []string{"Mixed Case Moddn"}, result.Entries[0].GetEqualFoldAttributeValues("cn"))
	})

	t.Run("requires delete permission", func(t *testing.T) {
		restricted := newTestServerWithACL(t, []string{"*::bind read add modify:*:*"})
		conn := restricted.dialRoot(t)

		dn := "uid=restricted-moddn," + peopleDN

		require.NoError(t, conn.Add(newUserAddRequest(dn, "restricted-moddn", "Restricted User")))

		err := conn.ModifyDN(ldap.NewModifyDNRequest(dn, "uid=restricted-moddn-renamed", true, ""))
		requireLDAPError(t, err, ldap.LDAPResultInsufficientAccessRights)
	})

	t.Run("missing entry", func(t *testing.T) {
		conn := server.dialRoot(t)

		dn := "uid=missing-moddn," + peopleDN
		err := conn.ModifyDN(ldap.NewModifyDNRequest(dn, "uid=missing-moddn", true, ""))
		requireLDAPError(t, err, ldap.LDAPResultNoSuchObject)
	})

	t.Run("superior outside naming context", func(t *testing.T) {
		conn := server.dialRoot(t)

		dn := "uid=outside-moddn," + peopleDN

		require.NoError(t, conn.Add(newUserAddRequest(dn, "outside-moddn", "Outside User")))
		err := conn.ModifyDN(ldap.NewModifyDNRequest(dn, "uid=outside-moddn", true, "dc=other,dc=test"))
		requireLDAPError(t, err, ldap.LDAPResultInvalidDNSyntax)
	})

	t.Run("move below itself", func(t *testing.T) {
		conn := server.dialRoot(t)

		dn := "uid=self-moddn," + peopleDN

		require.NoError(t, conn.Add(newUserAddRequest(dn, "self-moddn", "Self User")))
		err := conn.ModifyDN(ldap.NewModifyDNRequest(dn, "uid=self-moddn", true, dn))
		requireLDAPError(t, err, ldap.LDAPResultUnwillingToPerform)
	})

	t.Run("nested group reference", func(t *testing.T) {
		conn := server.dialRoot(t)

		parentGroup := "cn=parent-group-moddn," + groupsDN
		childGroup := "cn=child-group-moddn," + groupsDN
		newChildGroup := "cn=child-group-moddn-renamed," + groupsDN

		require.NoError(t, conn.Add(groupAddRequest(childGroup, "child-group-moddn", aliceDN)))
		require.NoError(t, conn.Add(groupAddRequest(parentGroup, "parent-group-moddn", childGroup)))
		require.NoError(t, conn.ModifyDN(ldap.NewModifyDNRequest(childGroup, "cn=child-group-moddn-renamed", true, "")))

		parent := search(t, conn, parentGroup, ldap.ScopeBaseObject, "(objectClass=*)", []string{"member"})
		require.Len(t, parent.Entries, 1)
		assert.Equal(t, newChildGroup, parent.Entries[0].GetAttributeValue("member"))

		alice := search(t, conn, aliceDN, ldap.ScopeBaseObject, "(objectClass=*)", []string{"memberOf"})
		require.Len(t, alice.Entries, 1)
		assert.Contains(t, alice.Entries[0].GetEqualFoldAttributeValues("memberOf"), newChildGroup)
	})
}
