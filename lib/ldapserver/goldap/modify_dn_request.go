package message

//	ModifyDNRequest ::= [APPLICATION 12] SEQUENCE {
//	     entry           LDAPDN,
//	     newrdn          RelativeLDAPDN,
//	     deleteoldrdn    BOOLEAN,
//	     newSuperior     [0] LDAPDN OPTIONAL }
func readModifyDNRequest(bytes *Bytes) (ret ModifyDNRequest, err error) {
	err = bytes.ReadSubBytes(classApplication, TagModifyDNRequest, ret.readComponents)
	if err != nil {
		err = LdapError{"readModifyDNRequest:\n" + err.Error()}
		return
	}

	return
}

func (m *ModifyDNRequest) readComponents(bytes *Bytes) (err error) {
	m.entry, err = readLDAPDN(bytes)
	if err != nil {
		err = LdapError{"readComponents:\n" + err.Error()}
		return err
	}

	m.newrdn, err = readRelativeLDAPDN(bytes)
	if err != nil {
		err = LdapError{"readComponents:\n" + err.Error()}
		return err
	}

	m.deleteoldrdn, err = readBOOLEAN(bytes)
	if err != nil {
		err = LdapError{"readComponents:\n" + err.Error()}
		return err
	}

	if bytes.HasMoreData() {
		var tag TagAndLength

		tag, err = bytes.PreviewTagAndLength()
		if err != nil {
			err = LdapError{"readComponents:\n" + err.Error()}
			return err
		}

		if tag.Tag == TagModifyDNRequestNewSuperior {
			var ldapdn LDAPDN

			ldapdn, err = readTaggedLDAPDN(bytes, classContextSpecific, TagModifyDNRequestNewSuperior)
			if err != nil {
				err = LdapError{"readComponents:\n" + err.Error()}
				return err
			}

			m.newSuperior = ldapdn.Pointer()
		}
	}

	return err
}

//	ModifyDNRequest ::= [APPLICATION 12] SEQUENCE {
//	     entry           LDAPDN,
//	     newrdn          RelativeLDAPDN,
//	     deleteoldrdn    BOOLEAN,
//	     newSuperior     [0] LDAPDN OPTIONAL }
func (m ModifyDNRequest) write(bytes *Bytes) (size int) {
	if m.newSuperior != nil {
		size += m.newSuperior.writeTagged(bytes, classContextSpecific, TagModifyDNRequestNewSuperior)
	}

	size += m.deleteoldrdn.write(bytes)
	size += m.newrdn.write(bytes)
	size += m.entry.write(bytes)
	size += bytes.WriteTagAndLength(classApplication, isCompound, TagModifyDNRequest, size)

	return
}

//	ModifyDNRequest ::= [APPLICATION 12] SEQUENCE {
//	     entry           LDAPDN,
//	     newrdn          RelativeLDAPDN,
//	     deleteoldrdn    BOOLEAN,
//	     newSuperior     [0] LDAPDN OPTIONAL }
func (m ModifyDNRequest) size() (size int) {
	size += m.entry.size()
	size += m.newrdn.size()

	size += m.deleteoldrdn.size()
	if m.newSuperior != nil {
		size += m.newSuperior.sizeTagged(TagModifyDNRequestNewSuperior)
	}

	size += sizeTagAndLength(TagModifyDNRequest, size)

	return
}
