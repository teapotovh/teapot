package message

import "fmt"

//	SearchResultEntry ::= [APPLICATION 4] SEQUENCE {
//	     objectName      LDAPDN,
//	     attributes      PartialAttributeList }
func readSearchResultEntry(bytes *Bytes) (searchresultentry SearchResultEntry, err error) {
	err = bytes.ReadSubBytes(classApplication, TagSearchResultEntry, searchresultentry.readComponents)
	if err != nil {
		err = LdapError{fmt.Sprintf("readSearchResultEntry:\n%s", err.Error())}
		return
	}
	return
}

func (s *SearchResultEntry) readComponents(bytes *Bytes) (err error) {
	s.objectName, err = readLDAPDN(bytes)
	if err != nil {
		err = LdapError{fmt.Sprintf("readComponents:\n%s", err.Error())}
		return
	}
	s.attributes, err = readPartialAttributeList(bytes)
	if err != nil {
		err = LdapError{fmt.Sprintf("readComponents:\n%s", err.Error())}
		return
	}
	return
}

//	SearchResultEntry ::= [APPLICATION 4] SEQUENCE {
//	     objectName      LDAPDN,
//	     attributes      PartialAttributeList }
func (s SearchResultEntry) write(bytes *Bytes) (size int) {
	size += s.attributes.write(bytes)
	size += s.objectName.write(bytes)
	size += bytes.WriteTagAndLength(classApplication, isCompound, TagSearchResultEntry, size)
	return
}

//	SearchResultEntry ::= [APPLICATION 4] SEQUENCE {
//	     objectName      LDAPDN,
//	     attributes      PartialAttributeList }
func (s SearchResultEntry) size() (size int) {
	size += s.objectName.size()
	size += s.attributes.size()
	size += sizeTagAndLength(tagSequence, size)
	return
}

func (s *SearchResultEntry) SetObjectName(on string) {
	s.objectName = LDAPDN(on)
}

func (s *SearchResultEntry) AddAttribute(name AttributeDescription, values ...AttributeValue) {
	ea := PartialAttribute{type_: name, vals: values}
	s.attributes.add(ea)
}
