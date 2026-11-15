package goldap

import (
	"testing"

	ldap "github.com/go-ldap/ldap/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRFC4515FilterExamples decodes the filter examples from RFC 4515 section
// 4 and checks that the BER decoder produces the expected filter structure.
// The filter strings are compiled with go-ldap, an independent LDAP client
// implementation, so this also checks interoperability of the wire format.
func TestRFC4515FilterExamples(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		filter string
		check  func(t *testing.T, filter Filter)
	}{
		{
			name:   "equality",
			filter: "(cn=Babs Jensen)",
			check: func(t *testing.T, filter Filter) {
				t.Helper()

				equality, ok := filter.(FilterEqualityMatch)
				require.True(t, ok, "got %T", filter)
				assert.Equal(t, AttributeDescription("cn"), equality.AttributeDesc())
				assert.Equal(t, AssertionValue("Babs Jensen"), equality.AssertionValue())
			},
		},
		{
			name:   "not",
			filter: "(!(cn=Tim Howes))",
			check: func(t *testing.T, filter Filter) {
				t.Helper()

				not, ok := filter.(FilterNot)
				require.True(t, ok, "got %T", filter)

				equality, ok := not.Filter.(FilterEqualityMatch)
				require.True(t, ok, "got %T", not.Filter)
				assert.Equal(t, AttributeDescription("cn"), equality.AttributeDesc())
				assert.Equal(t, AssertionValue("Tim Howes"), equality.AssertionValue())
			},
		},
		{
			name:   "and or and substring",
			filter: "(&(objectClass=Person)(|(sn=Jensen)(cn=Babs J*)))",
			check: func(t *testing.T, filter Filter) {
				t.Helper()

				and, ok := filter.(FilterAnd)
				require.True(t, ok, "got %T", filter)
				require.Len(t, and, 2)

				objectClass, ok := and[0].(FilterEqualityMatch)
				require.True(t, ok, "got %T", and[0])
				assert.Equal(t, AttributeDescription("objectClass"), objectClass.AttributeDesc())
				assert.Equal(t, AssertionValue("Person"), objectClass.AssertionValue())

				or, ok := and[1].(FilterOr)
				require.True(t, ok, "got %T", and[1])
				require.Len(t, or, 2)

				sn, ok := or[0].(FilterEqualityMatch)
				require.True(t, ok, "got %T", or[0])
				assert.Equal(t, AttributeDescription("sn"), sn.AttributeDesc())
				assert.Equal(t, AssertionValue("Jensen"), sn.AssertionValue())

				cn, ok := or[1].(FilterSubstrings)
				require.True(t, ok, "got %T", or[1])
				assert.Equal(t, AttributeDescription("cn"), cn.Type_())
				require.NotEmpty(t, cn.Substrings())
				assert.Equal(t, SubstringInitial("Babs J"), cn.Substrings()[0])
			},
		},
		{
			name:   "substring",
			filter: "(o=univ*of*mich*)",
			check: func(t *testing.T, filter Filter) {
				t.Helper()

				substrings, ok := filter.(FilterSubstrings)
				require.True(t, ok, "got %T", filter)
				assert.Equal(t, AttributeDescription("o"), substrings.Type_())

				values := []string{}

				for _, substring := range substrings.Substrings() {
					switch value := substring.(type) {
					case SubstringInitial:
						values = append(values, string(value))
					case SubstringAny:
						values = append(values, string(value))
					case SubstringFinal:
						values = append(values, string(value))
					default:
						t.Fatalf("unexpected substring type %T", substring)
					}
				}

				assert.Equal(t, []string{"univ", "of", "mich"}, values)
			},
		},
		{
			name:   "present",
			filter: "(seeAlso=*)",
			check: func(t *testing.T, filter Filter) {
				t.Helper()

				present, ok := filter.(FilterPresent)
				require.True(t, ok, "got %T", filter)
				assert.Equal(t, FilterPresent("seeAlso"), present)
			},
		},
		{
			name:   "greater or equal",
			filter: "(uidNumber>=1000)",
			check: func(t *testing.T, filter Filter) {
				t.Helper()

				greater, ok := filter.(FilterGreaterOrEqual)
				require.True(t, ok, "got %T", filter)
				assert.Equal(t, AttributeDescription("uidNumber"), greater.AttributeDesc())
				assert.Equal(t, AssertionValue("1000"), greater.AssertionValue())
			},
		},
		{
			name:   "less or equal",
			filter: "(uidNumber<=1000)",
			check: func(t *testing.T, filter Filter) {
				t.Helper()

				less, ok := filter.(FilterLessOrEqual)
				require.True(t, ok, "got %T", filter)
				assert.Equal(t, AttributeDescription("uidNumber"), less.AttributeDesc())
				assert.Equal(t, AssertionValue("1000"), less.AssertionValue())
			},
		},
		{
			name:   "approximate",
			filter: "(cn~=Babs Jensen)",
			check: func(t *testing.T, filter Filter) {
				t.Helper()

				approx, ok := filter.(FilterApproxMatch)
				require.True(t, ok, "got %T", filter)
				assert.Equal(t, AttributeDescription("cn"), approx.AttributeDesc())
				assert.Equal(t, AssertionValue("Babs Jensen"), approx.AssertionValue())
			},
		},
		{
			name:   "extensible with matching rule",
			filter: "(cn:caseExactMatch:=Fred Flintstone)",
			check: func(t *testing.T, filter Filter) {
				t.Helper()

				_, ok := filter.(FilterExtensibleMatch)
				require.True(t, ok, "got %T", filter)
			},
		},
		{
			name:   "extensible with type only",
			filter: "(cn:=Betty Rubble)",
			check: func(t *testing.T, filter Filter) {
				t.Helper()

				_, ok := filter.(FilterExtensibleMatch)
				require.True(t, ok, "got %T", filter)
			},
		},
		{
			name:   "extensible with dn attributes",
			filter: "(sn:dn:2.4.6.8.10:=Barney Rubble)",
			check: func(t *testing.T, filter Filter) {
				t.Helper()

				_, ok := filter.(FilterExtensibleMatch)
				require.True(t, ok, "got %T", filter)
			},
		},
		{
			name:   "extensible with no type",
			filter: "(:1.2.3:=Wilma Flintstone)",
			check: func(t *testing.T, filter Filter) {
				t.Helper()

				_, ok := filter.(FilterExtensibleMatch)
				require.True(t, ok, "got %T", filter)
			},
		},
		{
			name:   "extensible with no type and dn attributes",
			filter: "(:DN:2.4.6.8.10:=Dino)",
			check: func(t *testing.T, filter Filter) {
				t.Helper()

				_, ok := filter.(FilterExtensibleMatch)
				require.True(t, ok, "got %T", filter)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			packet, err := ldap.CompileFilter(test.filter)
			require.NoError(t, err)

			filter, err := readFilter(NewBytes(0, packet.Bytes()))
			require.NoError(t, err)

			test.check(t, filter)
		})
	}
}
