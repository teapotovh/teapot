package ldapserver

import ldap "github.com/teapotovh/teapot/lib/ldapserver/goldap"

func NewBindResponse(resultCode int32) ldap.BindResponse {
	r := ldap.BindResponse{}
	r.SetResultCode(resultCode)

	return r
}

func NewResponse(resultCode int32) ldap.LDAPResult {
	r := ldap.LDAPResult{}
	r.SetResultCode(resultCode)

	return r
}

func NewExtendedResponse(resultCode int32) ldap.ExtendedResponse {
	r := ldap.ExtendedResponse{}
	r.SetResultCode(resultCode)

	return r
}

func NewCompareResponse(resultCode int32) ldap.CompareResponse {
	r := ldap.CompareResponse{}
	r.SetResultCode(resultCode)

	return r
}

func NewModifyResponse(resultCode int32) ldap.ModifyResponse {
	r := ldap.ModifyResponse{}
	r.SetResultCode(resultCode)

	return r
}

func NewDeleteResponse(resultCode int32) ldap.DelResponse {
	r := ldap.DelResponse{}
	r.SetResultCode(resultCode)

	return r
}

func NewAddResponse(resultCode int32) ldap.AddResponse {
	r := ldap.AddResponse{}
	r.SetResultCode(resultCode)

	return r
}

func NewSearchResultDoneResponse(resultCode int32) ldap.SearchResultDone {
	r := ldap.SearchResultDone{}
	r.SetResultCode(resultCode)

	return r
}

func NewSearchResultEntry(objectname string) ldap.SearchResultEntry {
	r := ldap.SearchResultEntry{}
	r.SetObjectName(objectname)

	return r
}
