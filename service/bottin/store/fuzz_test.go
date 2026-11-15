package store

import (
	"testing"
)

// FuzzParseDN checks that the DN and prefix parsers never panic on arbitrary
// input. The seeds are the RFC 4514 section 3 examples.
func FuzzParseDN(f *testing.F) {
	for _, raw := range []string{
		"CN=Steve Kille,O=Isode Limited,C=GB",
		"OU=Sales+CN=J. Smith,O=Widget Inc.,C=US",
		`CN=L. Eagle,O=Sue\, Grabbit and Runn,C=GB`,
		`CN=Before\0DAfter,O=Test,C=GB`,
		`SN=Lu\C4\8Di\C4\87`,
		"CN=Steve Kille, O=Isode Limited, C=GB",
		"dc=example,dc=com",
		"",
		"CN",
		"CN=a=b",
	} {
		f.Add(raw)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		dn, err := ParseDN(raw)
		if err == nil {
			_ = dn.String()
			_ = dn.Prefix()
			_ = dn.Parent()
		}

		_, _ = ParsePrefix(raw)
	})
}
