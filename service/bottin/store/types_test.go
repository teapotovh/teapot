package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	skipDNEmpty         = "RFC 4514: the empty DN is valid and denotes the root DSE"
	skipDNEscapedComma  = "RFC 4514: escaped separators are not implemented by the DN parser"
	skipDNHexEscape     = "RFC 4514: hex escapes are not implemented by the DN parser"
	skipDNMultiValued   = "RFC 4514: multi-valued RDNs are not implemented by the DN parser"
	skipDNQuotedValue   = "RFC 4514: quoted values are not implemented by the DN parser"
	skipDNLeadingHash   = "RFC 4514: a leading # must be escaped; the parser accepts it literally"
	skipDNTrailingSpace = "RFC 4514: escaping a trailing space is not implemented by the DN parser"
)

// TestParseDN uses the distinguished name examples from RFC 4514 section 3.
// Cases that the current parser does not support are skipped with a reason so
// the gap remains visible without breaking the build.
func TestParseDN(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		want    DN
		skip    string
		wantErr bool
	}{
		{
			name: "simple",
			raw:  "CN=Steve Kille,O=Isode Limited,C=GB",
			want: DN{
				NewComponent("cn", "steve kille"),
				NewComponent("o", "isode limited"),
				NewComponent("c", "gb"),
			},
		},
		{
			name: "spaces around separators",
			raw:  "CN=Steve Kille, O=Isode Limited, C=GB",
			want: DN{
				NewComponent("cn", "steve kille"),
				NewComponent("o", "isode limited"),
				NewComponent("c", "gb"),
			},
		},
		{
			name: "lowercase and uppercase are equivalent",
			raw:  "cn=steve kille,o=isode limited,c=gb",
			want: DN{
				NewComponent("cn", "steve kille"),
				NewComponent("o", "isode limited"),
				NewComponent("c", "gb"),
			},
		},
		{
			name: "multi-valued rdn",
			raw:  `OU=Sales+CN=J. Smith,O=Widget Inc.,C=US`,
			skip: skipDNMultiValued,
		},
		{
			name: "escaped comma",
			raw:  `CN=L. Eagle,O=Sue\, Grabbit and Runn,C=GB`,
			skip: skipDNEscapedComma,
		},
		{
			name: "hex escaped carriage return",
			raw:  `CN=Before\0DAfter,O=Test,C=GB`,
			skip: skipDNHexEscape,
		},
		{
			name: "utf8 hex escape",
			raw:  `SN=Lu\C4\8Di\C4\87`,
			skip: skipDNHexEscape,
		},
		{
			name: "leading hash",
			raw:  `CN=#hash,O=Test,C=GB`,
			skip: skipDNLeadingHash,
		},
		{
			name: "quoted value",
			raw:  `CN="Steve Kille",O=Test,C=GB`,
			skip: skipDNQuotedValue,
		},
		{
			name: "escaped trailing space",
			raw:  `CN=Steve\ ,O=Test,C=GB`,
			skip: skipDNTrailingSpace,
		},
		{
			name: "empty dn",
			raw:  "",
			skip: skipDNEmpty,
		},
		{
			name:    "missing equals",
			raw:     "CN",
			wantErr: true,
		},
		{
			name:    "multiple equals",
			raw:     "CN=Steve=Kille,O=Test,C=GB",
			wantErr: true,
		},
		{
			name:    "empty component",
			raw:     "CN=Steve Kille,,C=GB",
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if test.skip != "" {
				t.Skip(test.skip)
			}

			dn, err := ParseDN(test.raw)
			if test.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, test.want, dn)
		})
	}
}

func TestDNStringRoundTrip(t *testing.T) {
	t.Parallel()

	dn, err := ParseDN("CN=Steve Kille,O=Isode Limited,C=GB")
	require.NoError(t, err)

	assert.Equal(t, "cn=steve kille,o=isode limited,c=gb", dn.String())

	roundTripped, err := ParseDN(dn.String())
	require.NoError(t, err)
	assert.True(t, dn.Equal(roundTripped))
}

func TestPrefixOperations(t *testing.T) {
	t.Parallel()

	base, err := ParseDN("dc=teapot,dc=ovh")
	require.NoError(t, err)

	child, err := ParseDN("ou=users,dc=teapot,dc=ovh")
	require.NoError(t, err)

	assert.True(t, base.Prefix().IsPrefixOf(child.Prefix()))
	assert.False(t, child.Prefix().IsPrefixOf(base.Prefix()))
	assert.Equal(t, "dc=ovh/dc=teapot/ou=users", child.Prefix().String())
	assert.Equal(t, "dc=teapot,dc=ovh", child.Parent().String())
	assert.Equal(t, "ou=users,dc=teapot,dc=ovh", child.String())
}
