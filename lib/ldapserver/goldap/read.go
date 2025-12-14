package message

func ReadLDAPMessage(bytes *Bytes) (message LDAPMessage, err error) {
	err = bytes.ReadSubBytes(classUniversal, tagSequence, message.readComponents)
	if err != nil {
		err = LdapError{"ReadLDAPMessage:\n" + err.Error()}
		return
	}
	return
}

//
//        END
//
