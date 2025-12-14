package message

import "fmt"

//
//        ExtendedResponse ::= [APPLICATION 24] SEQUENCE {
//             COMPONENTS OF LDAPResult,
//             responseName     [10] LDAPOID OPTIONAL,
//             responseValue    [11] OCTET STRING OPTIONAL }

func (extended *ExtendedResponse) SetResponseName(name LDAPOID) {
	extended.responseName = &name
}

func readExtendedResponse(bytes *Bytes) (ret ExtendedResponse, err error) {
	err = bytes.ReadSubBytes(classApplication, TagExtendedResponse, ret.readComponents)
	if err != nil {
		err = LdapError{"readExtendedResponse:\n" + err.Error()}
		return
	}
	return
}

func (extended *ExtendedResponse) readComponents(bytes *Bytes) (err error) {
	if err := extended.LDAPResult.readComponents(bytes); err != nil {
		return fmt.Errorf("error while reading LDAP result: %w", err)
	}
	if bytes.HasMoreData() {
		var tag TagAndLength
		tag, err = bytes.PreviewTagAndLength()
		if err != nil {
			err = LdapError{"readComponents:\n" + err.Error()}
			return err
		}
		if tag.Tag == TagExtendedResponseName {
			var oid LDAPOID
			oid, err = readTaggedLDAPOID(bytes, classContextSpecific, TagExtendedResponseName)
			if err != nil {
				err = LdapError{"readComponents:\n" + err.Error()}
				return err
			}
			extended.responseName = oid.Pointer()
		}
	}
	if bytes.HasMoreData() {
		var tag TagAndLength
		tag, err = bytes.PreviewTagAndLength()
		if err != nil {
			err = LdapError{"readComponents:\n" + err.Error()}
			return err
		}
		if tag.Tag == TagExtendedResponseValue {
			var responseValue OCTETSTRING
			responseValue, err = readTaggedOCTETSTRING(bytes, classContextSpecific, TagExtendedResponseValue)
			if err != nil {
				err = LdapError{"readComponents:\n" + err.Error()}
				return err
			}
			extended.responseValue = responseValue.Pointer()
		}
	}
	return err
}

func (extended ExtendedResponse) write(bytes *Bytes) (size int) {
	if extended.responseValue != nil {
		size += extended.responseValue.writeTagged(bytes, classContextSpecific, TagExtendedResponseValue)
	}
	if extended.responseName != nil {
		size += extended.responseName.writeTagged(bytes, classContextSpecific, TagExtendedResponseName)
	}
	size += extended.writeComponents(bytes)
	size += bytes.WriteTagAndLength(classApplication, isCompound, TagExtendedResponse, size)
	return
}

func (extended ExtendedResponse) size() (size int) {
	size += extended.sizeComponents()
	if extended.responseName != nil {
		size += extended.responseName.sizeTagged(TagExtendedResponseName)
	}
	if extended.responseValue != nil {
		size += extended.responseValue.sizeTagged(TagExtendedResponseValue)
	}
	size += sizeTagAndLength(TagExtendedResponse, size)
	return
}
