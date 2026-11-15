package goldap

import (
	"testing"

	ldap "github.com/go-ldap/ldap/v3"
)

// FuzzReadLDAPMessage checks that the BER decoder never panics on arbitrary
// input. Run it with:
//
//	bazel test //lib/ldapsrv/goldap:goldap_test \
//	  --test_arg=-test.run=^$ \
//	  --test_arg=-test.fuzz=FuzzReadLDAPMessage \
//	  --test_arg=-test.fuzztime=30s \
//	  --test_arg=-test.fuzzcachedir=/tmp/fuzzcache
func FuzzReadLDAPMessage(f *testing.F) {
	for _, test := range getLDAPMessageTestData() {
		f.Add(test.bytes.getBytes())
	}

	f.Add([]byte{0x30, 0x00})
	f.Add([]byte{0x30, 0x84, 0xff, 0xff, 0xff, 0xff})
	f.Add([]byte{0x30, 0x03, 0x02, 0x01, 0x01})

	f.Fuzz(func(t *testing.T, data []byte) {
		message, err := ReadLDAPMessage(NewBytes(0, data))
		if err != nil {
			return
		}

		// A message that decodes must also encode without panicking. The
		// encoded form does not have to be byte-identical to the input,
		// because BER permits non-canonical encodings.
		if _, err := message.Write(); err != nil {
			t.Fatalf("decoded message could not be encoded: %v", err)
		}
	})
}

// FuzzReadFilter checks the search filter decoder with the RFC 4515 examples
// and arbitrary input.
func FuzzReadFilter(f *testing.F) {
	for _, raw := range []string{
		"(cn=Babs Jensen)",
		"(!(cn=Tim Howes))",
		"(&(objectClass=Person)(|(sn=Jensen)(cn=Babs J*)))",
		"(o=univ*of*mich*)",
		"(seeAlso=*)",
		"(uidNumber>=1000)",
		"(cn~=Babs Jensen)",
		"(cn:caseExactMatch:=Fred Flintstone)",
		"(:1.2.3:=Wilma Flintstone)",
	} {
		packet, err := ldap.CompileFilter(raw)
		if err != nil {
			continue
		}

		f.Add(packet.Bytes())
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = readFilter(NewBytes(0, data))
	})
}
