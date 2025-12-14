package message

// present         [7] AttributeDescription,.
func readFilterPresent(bytes *Bytes) (ret FilterPresent, err error) {
	var attributedescription AttributeDescription

	attributedescription, err = readTaggedAttributeDescription(bytes, classContextSpecific, TagFilterPresent)
	if err != nil {
		err = LdapError{"readFilterPresent:\n" + err.Error()}
		return
	}

	ret = FilterPresent(attributedescription)

	return
}

// present         [7] AttributeDescription,.
func (f FilterPresent) write(bytes *Bytes) int {
	return AttributeDescription(f).writeTagged(bytes, classContextSpecific, TagFilterPresent)
}

func (f FilterPresent) getFilterTag() int {
	return TagFilterPresent
}

// present         [7] AttributeDescription,.
func (f FilterPresent) size() int {
	return AttributeDescription(f).sizeTagged(TagFilterPresent)
}
