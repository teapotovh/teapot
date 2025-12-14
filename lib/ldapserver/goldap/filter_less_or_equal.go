package message

// lessOrEqual     [6] AttributeValueAssertion,.
func readFilterLessOrEqual(bytes *Bytes) (ret FilterLessOrEqual, err error) {
	var attributevalueassertion AttributeValueAssertion
	attributevalueassertion, err = readTaggedAttributeValueAssertion(bytes, classContextSpecific, TagFilterLessOrEqual)
	if err != nil {
		err = LdapError{"readFilterLessOrEqual:\n" + err.Error()}
		return
	}
	ret = FilterLessOrEqual(attributevalueassertion)
	return
}

// lessOrEqual     [6] AttributeValueAssertion,.
func (f FilterLessOrEqual) write(bytes *Bytes) int {
	return AttributeValueAssertion(f).writeTagged(bytes, classContextSpecific, TagFilterLessOrEqual)
}

func (f FilterLessOrEqual) getFilterTag() int {
	return TagFilterLessOrEqual
}

// lessOrEqual     [6] AttributeValueAssertion,.
func (f FilterLessOrEqual) size() int {
	return AttributeValueAssertion(f).sizeTagged(TagFilterLessOrEqual)
}

func (f *FilterLessOrEqual) AttributeDesc() AttributeDescription {
	return f.attributeDesc
}

func (f *FilterLessOrEqual) AssertionValue() AssertionValue {
	return f.assertionValue
}
