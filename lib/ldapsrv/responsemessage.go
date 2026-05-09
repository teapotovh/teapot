package ldapsrv

import ldap "github.com/teapotovh/teapot/lib/ldapsrv/goldap"

func NewBindResponse(resultCode ldap.ENUMERATED) ldap.BindResponse {
	r := ldap.BindResponse{}
	r.SetResultCode(resultCode)

	return r
}

func NewResponse(resultCode ldap.ENUMERATED) ldap.LDAPResult {
	r := ldap.LDAPResult{}
	r.SetResultCode(resultCode)

	return r
}

func NewExtendedResponse(resultCode ldap.ENUMERATED) ldap.ExtendedResponse {
	r := ldap.ExtendedResponse{}
	r.SetResultCode(resultCode)

	return r
}

func NewCompareResponse(resultCode ldap.ENUMERATED) ldap.CompareResponse {
	r := ldap.CompareResponse{}
	r.SetResultCode(resultCode)

	return r
}

func NewModifyResponse(resultCode ldap.ENUMERATED) ldap.ModifyResponse {
	r := ldap.ModifyResponse{}
	r.SetResultCode(resultCode)

	return r
}

func NewDeleteResponse(resultCode ldap.ENUMERATED) ldap.DelResponse {
	r := ldap.DelResponse{}
	r.SetResultCode(resultCode)

	return r
}

func NewAddResponse(resultCode ldap.ENUMERATED) ldap.AddResponse {
	r := ldap.AddResponse{}
	r.SetResultCode(resultCode)

	return r
}

func NewSearchResultDoneResponse(resultCode ldap.ENUMERATED) ldap.SearchResultDone {
	r := ldap.SearchResultDone{}
	r.SetResultCode(resultCode)

	return r
}

func NewSearchResultEntry(objectname string) ldap.SearchResultEntry {
	r := ldap.SearchResultEntry{}
	r.SetObjectName(objectname)

	return r
}
