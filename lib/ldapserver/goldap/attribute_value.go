package message

//
//        AttributeValue ::= OCTET STRING

func readAttributeValue(bytes *Bytes) (ret AttributeValue, err error) {
	octetstring, err := readOCTETSTRING(bytes)
	if err != nil {
		err = LdapError{"readAttributeValue:\n" + err.Error()}
		return
	}
	ret = AttributeValue(octetstring)
	return
}

func (value AttributeValue) write(bytes *Bytes) int {
	return OCTETSTRING(value).write(bytes)
}

func (value AttributeValue) size() int {
	return OCTETSTRING(value).size()
}
