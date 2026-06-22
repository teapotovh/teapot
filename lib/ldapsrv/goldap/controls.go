package message

//
//        Controls ::= SEQUENCE OF control Control

import "slices"

func readTaggedControls(bytes *Bytes, class int, tag int) (controls Controls, err error) {
	err = bytes.ReadSubBytes(class, tag, controls.readComponents)
	if err != nil {
		err = LdapError{"readTaggedControls:\n" + err.Error()}
		return
	}

	return
}

func (controls *Controls) readComponents(bytes *Bytes) (err error) {
	for bytes.HasMoreData() {
		var control Control

		control, err = readControl(bytes)
		if err != nil {
			err = LdapError{"readComponents:\n" + err.Error()}
			return
		}

		*controls = append(*controls, control)
	}

	return
}
func (controls Controls) Pointer() *Controls { return &controls }

func (controls Controls) writeTagged(bytes *Bytes, class int, tag int) (size int) {
	for _, v := range slices.Backward(controls) {
		size += v.write(bytes)
	}

	size += bytes.WriteTagAndLength(class, isCompound, tag, size)

	return
}

func (controls Controls) sizeTagged(tag int) (size int) {
	for _, control := range controls {
		size += control.size()
	}

	size += sizeTagAndLength(tag, size)

	return
}
