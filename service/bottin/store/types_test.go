package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseDN uses the distinguished name examples from RFC 4514 section 4.
// Multi-valued RDNs are not supported by design, so that case must fail.
func TestParseDN(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		want    DN
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
			name:    "multi-valued rdn",
			raw:     `OU=Sales+CN=J. Smith,O=Widget Inc.,C=US`,
			wantErr: true,
		},
		{
			name: "escaped comma",
			raw:  `CN=L. Eagle,O=Sue\, Grabbit and Runn,C=GB`,
			want: DN{
				NewComponent("cn", "l. eagle"),
				NewComponent("o", "sue, grabbit and runn"),
				NewComponent("c", "gb"),
			},
		},
		{
			name: "hex escaped carriage return",
			raw:  `CN=Before\0DAfter,O=Test,C=GB`,
			want: DN{
				NewComponent("cn", "before\rafter"),
				NewComponent("o", "test"),
				NewComponent("c", "gb"),
			},
		},
		{
			name: "utf8 hex escape",
			raw:  `SN=Lu\C4\8Di\C4\87`,
			want: DN{
				NewComponent("sn", "lu\u010di\u0107"),
			},
		},
		{
			name:    "leading hash",
			raw:     `CN=#hash,O=Test,C=GB`,
			wantErr: true,
		},
		{
			name: "escaped leading hash",
			raw:  `CN=\#hash,O=Test,C=GB`,
			want: DN{
				NewComponent("cn", "#hash"),
				NewComponent("o", "test"),
				NewComponent("c", "gb"),
			},
		},
		{
			name: "escaped quotes and comma",
			raw:  `CN=James \"Jim\" Smith\, III,DC=example,DC=net`,
			want: DN{
				NewComponent("cn", `james "jim" smith, iii`),
				NewComponent("dc", "example"),
				NewComponent("dc", "net"),
			},
		},
		{
			name:    "quoted value",
			raw:     `CN="Steve Kille",O=Test,C=GB`,
			wantErr: true,
		},
		{
			name: "escaped trailing space",
			raw:  `CN=Steve\ ,O=Test,C=GB`,
			want: DN{
				NewComponent("cn", "steve "),
				NewComponent("o", "test"),
				NewComponent("c", "gb"),
			},
		},
		{
			name: "empty dn",
			raw:  "",
			want: DN{},
		},
		{
			name:    "missing equals",
			raw:     "CN",
			wantErr: true,
		},
		{
			name: "unescaped equals",
			raw:  "CN=Steve=Kille,O=Test,C=GB",
			want: DN{
				NewComponent("cn", "steve=kille"),
				NewComponent("o", "test"),
				NewComponent("c", "gb"),
			},
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

	tests := []string{
		`CN=L. Eagle,O=Sue\, Grabbit and Runn,C=GB`,
		`CN=James \"Jim\" Smith\, III,DC=example,DC=net`,
		`CN=Before\0DAfter,O=Test,C=GB`,
		`SN=Lu\C4\8Di\C4\87`,
		`CN=\#hash,O=Test,C=GB`,
		`CN=Steve\ ,O=Test,C=GB`,
		`CN=a\+b,O=Test,C=GB`,
		`CN=a\=b,O=Test,C=GB`,
	}

	for _, raw := range tests {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()

			dn, err := ParseDN(raw)
			require.NoError(t, err)

			roundTripped, err := ParseDN(dn.String())
			require.NoError(t, err)

			assert.True(t, dn.Equal(roundTripped), "want %q, got %q", dn, roundTripped)
		})
	}
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

	slash, err := ParseDN(`cn=a/b,dc=teapot,dc=ovh`)
	require.NoError(t, err)

	assert.Equal(t, `dc=ovh/dc=teapot/cn=a\2Fb`, slash.Prefix().String())

	prefix, err := ParsePrefix(slash.Prefix().String())
	require.NoError(t, err)
	assert.True(t, prefix.Equal(slash.Prefix()))
}
