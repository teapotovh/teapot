package message

// Referral ::= SEQUENCE SIZE (1..MAX) OF uri URI.
func readTaggedReferral(bytes *Bytes, class int, tag int) (referral Referral, err error) {
	err = bytes.ReadSubBytes(class, tag, referral.readComponents)
	if err != nil {
		err = LdapError{"readTaggedReferral:\n" + err.Error()}
		return
	}

	return
}

func (r *Referral) readComponents(bytes *Bytes) (err error) {
	for bytes.HasMoreData() {
		var uri URI

		uri, err = readURI(bytes)
		if err != nil {
			err = LdapError{"readComponents:\n" + err.Error()}
			return
		}

		*r = append(*r, uri)
	}

	if len(*r) == 0 {
		return LdapError{"readComponents: expecting at least one URI"}
	}

	return
}
func (r Referral) Pointer() *Referral { return &r }

// Referral ::= SEQUENCE SIZE (1..MAX) OF uri URI.
func (r Referral) writeTagged(bytes *Bytes, class int, tag int) (size int) {
	for i := len(r) - 1; i >= 0; i-- {
		size += r[i].write(bytes)
	}

	size += bytes.WriteTagAndLength(class, isCompound, tag, size)

	return
}

// Referral ::= SEQUENCE SIZE (1..MAX) OF uri URI.
func (r Referral) sizeTagged(tag int) (size int) {
	for _, uri := range r {
		size += uri.size()
	}

	size += sizeTagAndLength(tag, size)

	return
}
