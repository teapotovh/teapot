package message

//	PasswdModifyRequestValue ::= SEQUENCE {
//	  userIdentity    [0]  OCTET STRING OPTIONAL
//	  oldPasswd       [1]  OCTET STRING OPTIONAL
//	  newPasswd       [2]  OCTET STRING OPTIONAL }

func (request *PasswordModifyRequest) UserIdentity() *OCTETSTRING {
	return request.userIdentity
}

func (request *PasswordModifyRequest) OldPassword() *OCTETSTRING {
	return request.oldPassword
}

func (request *PasswordModifyRequest) NewPassword() *OCTETSTRING {
	return request.newPassword
}

func readPasswordModifyRequest(bytes *Bytes) (request PasswordModifyRequest, err error) {
	err = bytes.ReadSubBytes(classUniversal, tagSequence, request.readComponents)
	if err != nil {
		err = LdapError{"readPasswordModifyRequest:\n" + err.Error()}
		return
	}
	return
}

func (request *PasswordModifyRequest) readComponents(bytes *Bytes) (err error) {
	request.userIdentity, err = readOptionalOctetString(bytes, TagPasswordModifyRequestUserIdentity)
	if err != nil {
		return
	}

	request.oldPassword, err = readOptionalOctetString(bytes, TagPasswordModifyRequestOldPassword)
	if err != nil {
		return
	}

	request.newPassword, err = readOptionalOctetString(bytes, TagPasswordModifyRequestNewPassword)
	if err != nil {
		return
	}

	return
}

func readOptionalOctetString(bytes *Bytes, expectedTag int) (ptr *OCTETSTRING, err error) {
	if bytes.HasMoreData() {
		var tag TagAndLength
		tag, err = bytes.PreviewTagAndLength()
		if err != nil {
			err = LdapError{"readComponents:\n" + err.Error()}
			return
		}
		if tag.Tag == expectedTag {
			var serverSaslCreds OCTETSTRING
			serverSaslCreds, err = readTaggedOCTETSTRING(bytes, classContextSpecific, expectedTag)
			if err != nil {
				err = LdapError{"readComponents:\n" + err.Error()}
				return
			}
			ptr = serverSaslCreds.Pointer()
			return
		}
	}
	return
}

func (request PasswordModifyRequest) write(bytes *Bytes) (size int) {
	if request.userIdentity != nil {
		size += request.userIdentity.writeTagged(bytes, classContextSpecific, TagPasswordModifyRequestUserIdentity)
	}
	if request.oldPassword != nil {
		size += request.oldPassword.writeTagged(bytes, classContextSpecific, TagPasswordModifyRequestOldPassword)
	}
	if request.newPassword != nil {
		size += request.newPassword.writeTagged(bytes, classContextSpecific, TagPasswordModifyRequestNewPassword)
	}
	return
}

func (request PasswordModifyRequest) size() (size int) {
	if request.userIdentity != nil {
		size += request.userIdentity.sizeTagged(TagPasswordModifyRequestUserIdentity)
	}
	if request.oldPassword != nil {
		size += request.oldPassword.sizeTagged(TagPasswordModifyRequestOldPassword)
	}
	if request.newPassword != nil {
		size += request.newPassword.sizeTagged(TagPasswordModifyRequestNewPassword)
	}
	return
}
