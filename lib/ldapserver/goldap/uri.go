package message

// URI ::= LDAPString     -- limited to characters permitted in
//
//	-- URIs
func readURI(bytes *Bytes) (uri URI, err error) {
	var ldapstring LDAPString
	ldapstring, err = readLDAPString(bytes)
	// @TODO: check permitted chars in URI
	if err != nil {
		err = LdapError{"readURI:\n" + err.Error()}
		return
	}
	uri = URI(ldapstring)
	return
}

// URI ::= LDAPString     -- limited to characters permitted in
//
//	-- URIs
func (u URI) write(bytes *Bytes) int {
	return LDAPString(u).write(bytes)
}

// URI ::= LDAPString     -- limited to characters permitted in
//
//	-- URIs
func (u URI) size() int {
	return LDAPString(u).size()
}
