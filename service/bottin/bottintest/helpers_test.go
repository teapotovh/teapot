package bottintest

import (
	"github.com/go-ldap/ldap/v3"
)

const (
	skipBadVersion   = "RFC 4511 4.2.7: the BER decoder rejects the version before the handler can respond"
	skipSASLBind     = "RFC 4511 4.2.9: SASL is not implemented"
	skipMalformedPDU = "RFC 4511 4.1.1: Bottin keeps the connection open after a malformed PDU"
	skipNoSuchBase   = "RFC 4511 4.5.4: Bottin returns success for a missing search base"
	skipMatchedDN    = "RFC 4511 4.5.5: Bottin does not return a matchedDN"
	skipSizeLimit    = "RFC 4511 4.5.1: search size limits are not enforced"
	skipTypesOnly    = "RFC 4511 4.5.1: typesOnly is not implemented"
	skipDerefAliases = "RFC 4511 4.5.1: alias dereferencing is not implemented"
	skipSubstring    = "RFC 4511 4.5.1: substring filters are not implemented"
	skipModifyDN     = "RFC 4511 4.9: Modify DN is not implemented"
	skipWhoAmI       = "RFC 4532: Who Am I is not implemented" //nolint:gosec
	skipPagedResults = "RFC 2696: paged results are not implemented"
	skipOldPassword  = "RFC 3062: Bottin does not validate oldPasswd" //nolint:gosec
	skipOperational  = "RFC 3673: Bottin does not expose the full operational attribute set"
)

func newUserAddRequest(dn string, uid string, cn string) *ldap.AddRequest {
	request := ldap.NewAddRequest(dn, nil)
	request.Attributes = []ldap.Attribute{
		{Type: "objectClass", Vals: []string{"top", "person", "organizationalPerson", "inetOrgPerson"}},
		{Type: "uid", Vals: []string{uid}},
		{Type: "cn", Vals: []string{cn}},
		{Type: "sn", Vals: []string{"Test"}},
	}

	return request
}

func groupAddRequest(dn string, cn string, members ...string) *ldap.AddRequest {
	request := ldap.NewAddRequest(dn, nil)
	request.Attributes = []ldap.Attribute{
		{Type: "objectClass", Vals: []string{"top", "groupOfNames"}},
		{Type: "cn", Vals: []string{cn}},
		{Type: "member", Vals: members},
	}

	return request
}
