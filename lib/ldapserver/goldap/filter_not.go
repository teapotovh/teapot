package message

func (filterNot FilterNot) getFilterTag() int {
	return TagFilterNot
}

// not             [2] Filter,.
func (filterNot FilterNot) size() (size int) {
	size = filterNot.Filter.size()
	size += sizeTagAndLength(tagSequence, size)

	return
}

func (filterNot *FilterNot) readComponents(bytes *Bytes) (err error) {
	filterNot.Filter, err = readFilter(bytes)
	if err != nil {
		err = LdapError{"readComponents:\n" + err.Error()}
		return
	}

	return
}

// not             [2] Filter,.
func (filterNot FilterNot) write(bytes *Bytes) (size int) {
	size = filterNot.Filter.write(bytes)
	size += bytes.WriteTagAndLength(classContextSpecific, isCompound, TagFilterNot, size)

	return
}

// not             [2] Filter,.
func readFilterNot(bytes *Bytes) (filternot FilterNot, err error) {
	err = bytes.ReadSubBytes(classContextSpecific, TagFilterNot, filternot.readComponents)
	if err != nil {
		err = LdapError{"readFilterNot:\n" + err.Error()}
		return
	}

	return
}
