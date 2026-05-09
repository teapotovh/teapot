package message

// ModifyDNResponse ::= [APPLICATION 13] LDAPResult.
func readModifyDNResponse(bytes *Bytes) (ret ModifyDNResponse, err error) {
	var res LDAPResult

	res, err = readTaggedLDAPResult(bytes, classApplication, TagModifyDNResponse)
	if err != nil {
		err = LdapError{"readModifyDNResponse:\n" + err.Error()}
		return
	}

	ret = ModifyDNResponse(res)

	return
}

// ModifyDNResponse ::= [APPLICATION 13] LDAPResult.
func (m ModifyDNResponse) write(bytes *Bytes) int {
	return LDAPResult(m).writeTagged(bytes, classApplication, TagModifyDNResponse)
}

// ModifyDNResponse ::= [APPLICATION 13] LDAPResult.
func (m ModifyDNResponse) size() int {
	return LDAPResult(m).sizeTagged(TagModifyDNResponse)
}
