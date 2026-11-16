package bottintest

import (
	"testing"

	"github.com/go-ldap/ldap/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/teapotovh/teapot/lib/ldapsrv"
	"github.com/teapotovh/teapot/service/bottin"
)

// Adapted from:
// https://github.com/rumba-id/bauble/blob/ce079b46a8996ea0b8a1ec5bd07def580944579b/src/bauble/suites/rfc4511/search.py
func TestBaubleSearch(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	t.Run("base scope", func(t *testing.T) {
		t.Parallel()

		conn := server.dialRoot(t)

		result := search(t, conn, baseDN, ldap.ScopeBaseObject, "(objectClass=*)", nil)
		require.Len(t, result.Entries, 1)
		assert.Equal(t, baseDN, result.Entries[0].DN)
	})

	t.Run("single level", func(t *testing.T) {
		t.Parallel()

		conn := server.dialRoot(t)

		result := search(t, conn, peopleDN, ldap.ScopeSingleLevel, "(objectClass=*)", nil)
		require.GreaterOrEqual(t, len(result.Entries), 2)

		dns := []string{}
		for _, entry := range result.Entries {
			dns = append(dns, entry.DN)
		}

		assert.Contains(t, dns, aliceDN)
		assert.Contains(t, dns, bobDN)
	})

	t.Run("subtree filter", func(t *testing.T) {
		t.Parallel()

		conn := server.dialRoot(t)

		result := search(t, conn, baseDN, ldap.ScopeWholeSubtree, "(uid=alice)", nil)
		require.Len(t, result.Entries, 1)
		assert.Equal(t, aliceDN, result.Entries[0].DN)
	})

	t.Run("missing base", func(t *testing.T) {
		t.Parallel()
		t.Skip(skipNoSuchBase)
	})

	t.Run("matched dn", func(t *testing.T) {
		t.Parallel()
		t.Skip(skipMatchedDN)
	})

	t.Run("substring filter", func(t *testing.T) {
		t.Parallel()
		t.Skip(skipSubstring)
	})

	t.Run("size limit", func(t *testing.T) {
		t.Parallel()
		t.Skip(skipSizeLimit)
	})

	t.Run("types only", func(t *testing.T) {
		t.Parallel()
		t.Skip(skipTypesOnly)
	})

	t.Run("deref aliases", func(t *testing.T) {
		t.Parallel()
		t.Skip(skipDerefAliases)
	})
}

// Local test based on the filter examples in RFC 4515 section 4:
// https://www.rfc-editor.org/rfc/rfc4515#section-4
func TestBaubleSearchFilters(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	tests := []struct {
		name      string
		filter    string
		wantCount int
		check     func(t *testing.T, result *ldap.SearchResult)
	}{
		{
			name:      "equality",
			filter:    "(uid=alice)",
			wantCount: 1,
		},
		{
			name:      "case insensitive value",
			filter:    "(uid=ALICE)",
			wantCount: 1,
		},
		{
			name:      "and",
			filter:    "(&(uid=alice)(objectClass=*))",
			wantCount: 1,
		},
		{
			name:      "or",
			filter:    "(|(uid=alice)(uid=bob))",
			wantCount: 2,
		},
		{
			name:   "not",
			filter: "(!(uid=alice))",
			check: func(t *testing.T, result *ldap.SearchResult) {
				t.Helper()

				assert.NotEmpty(t, result.Entries)

				for _, entry := range result.Entries {
					assert.NotEqual(t, aliceDN, entry.DN)
				}
			},
		},
		{
			name:      "objectclass",
			filter:    "(objectClass=inetOrgPerson)",
			wantCount: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			conn := server.dialRoot(t)

			result := search(t, conn, baseDN, ldap.ScopeWholeSubtree, test.filter, nil)
			if test.check != nil {
				test.check(t, result)
				return
			}

			assert.Len(t, result.Entries, test.wantCount)
		})
	}
}

// Adapted from:
// https://github.com/rumba-id/bauble/blob/ce079b46a8996ea0b8a1ec5bd07def580944579b/src/bauble/suites/rfc4511/search.py
//
// The operational attribute case is from:
// https://github.com/rumba-id/bauble/blob/ce079b46a8996ea0b8a1ec5bd07def580944579b/src/bauble/suites/rfc3673.py
func TestBaubleSearchAttributeSelection(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	t.Run("named attribute", func(t *testing.T) {
		t.Parallel()

		conn := server.dialRoot(t)

		result := search(t, conn, aliceDN, ldap.ScopeBaseObject, "(objectClass=*)", []string{"cn"})
		require.Len(t, result.Entries, 1)

		entry := result.Entries[0]
		assert.Equal(t, "Alice Anderson", entry.GetAttributeValue("cn"))
		assert.Empty(t, entry.GetAttributeValue("mail"))
	})

	t.Run("no attributes", func(t *testing.T) {
		t.Parallel()

		conn := server.dialRoot(t)

		result := search(t, conn, aliceDN, ldap.ScopeBaseObject, "(objectClass=*)", []string{"1.1"})
		require.Len(t, result.Entries, 1)
		assert.Empty(t, result.Entries[0].Attributes)
	})

	t.Run("operational attributes", func(t *testing.T) {
		t.Parallel()
		t.Skip(skipOperational)
	})
}

// Adapted from:
// https://github.com/rumba-id/bauble/blob/ce079b46a8996ea0b8a1ec5bd07def580944579b/src/bauble/suites/rfc4512.py
func TestRootDSE(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	t.Run("base scope", func(t *testing.T) {
		t.Parallel()

		conn := server.dialRoot(t)

		result := search(t, conn, "", ldap.ScopeBaseObject, "(objectClass=*)", nil)
		require.Len(t, result.Entries, 1)

		entry := result.Entries[0]
		assert.Empty(t, entry.DN)
		assert.Equal(t, baseDN, entry.GetEqualFoldAttributeValue("namingContexts"))
		assert.Equal(t, "3", entry.GetEqualFoldAttributeValue("supportedLDAPVersion"))
		assert.Equal(
			t,
			string(ldapsrv.NoticeOfPasswordModify),
			entry.GetEqualFoldAttributeValue("supportedExtension"),
		)
	})

	t.Run("selected attribute", func(t *testing.T) {
		t.Parallel()

		conn := server.dialRoot(t)

		result := search(t, conn, "", ldap.ScopeBaseObject, "(objectClass=*)", []string{"namingContexts"})
		require.Len(t, result.Entries, 1)

		entry := result.Entries[0]
		assert.Equal(t, baseDN, entry.GetEqualFoldAttributeValue("namingContexts"))
		assert.Empty(t, entry.GetEqualFoldAttributeValue("supportedLDAPVersion"))
	})

	t.Run("no attributes", func(t *testing.T) {
		t.Parallel()

		conn := server.dialRoot(t)

		result := search(t, conn, "", ldap.ScopeBaseObject, "(objectClass=*)", []string{"1.1"})
		require.Len(t, result.Entries, 1)
		assert.Empty(t, result.Entries[0].Attributes)
	})

	t.Run("subtree scope", func(t *testing.T) {
		t.Parallel()

		conn := server.dialRoot(t)

		result := search(t, conn, "", ldap.ScopeWholeSubtree, "(objectClass=*)", nil)
		assert.Empty(t, result.Entries)
	})

	t.Run("user attributes", func(t *testing.T) {
		t.Parallel()

		conn := server.dialRoot(t)

		result := search(t, conn, "", ldap.ScopeBaseObject, "(objectClass=*)", []string{"*"})
		require.Len(t, result.Entries, 1)

		entry := result.Entries[0]
		assert.Equal(t, objectClassTop, entry.GetEqualFoldAttributeValue("objectClass"))
		assert.Empty(t, entry.GetEqualFoldAttributeValue("namingContexts"))
	})

	t.Run("operational attributes", func(t *testing.T) {
		t.Parallel()

		conn := server.dialRoot(t)

		result := search(t, conn, "", ldap.ScopeBaseObject, "(objectClass=*)", []string{"+"})
		require.Len(t, result.Entries, 1)

		entry := result.Entries[0]
		assert.Equal(t, baseDN, entry.GetEqualFoldAttributeValue("namingContexts"))
		assert.Empty(t, entry.GetEqualFoldAttributeValue("objectClass"))
	})

	t.Run("default acl", func(t *testing.T) {
		t.Parallel()

		defaultServer := newTestServerWithACL(t, bottin.DefaultACL)
		conn := defaultServer.dial(t)

		result := search(t, conn, "", ldap.ScopeBaseObject, "(objectClass=*)", []string{"namingContexts"})
		require.Len(t, result.Entries, 1)
		assert.Equal(t, baseDN, result.Entries[0].GetEqualFoldAttributeValue("namingContexts"))
	})
}
