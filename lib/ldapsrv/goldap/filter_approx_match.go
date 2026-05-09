package message

// approxMatch     [8] AttributeValueAssertion,.
func readFilterApproxMatch(bytes *Bytes) (ret FilterApproxMatch, err error) {
	var attributevalueassertion AttributeValueAssertion

	attributevalueassertion, err = readTaggedAttributeValueAssertion(bytes, classContextSpecific, TagFilterApproxMatch)
	if err != nil {
		err = LdapError{"readFilterApproxMatch:\n" + err.Error()}
		return
	}

	ret = FilterApproxMatch(attributevalueassertion)

	return
}

// approxMatch     [8] AttributeValueAssertion,.
func (f FilterApproxMatch) write(bytes *Bytes) int {
	return AttributeValueAssertion(f).writeTagged(bytes, classContextSpecific, TagFilterApproxMatch)
}

func (f FilterApproxMatch) getFilterTag() int {
	return TagFilterApproxMatch
}

// approxMatch     [8] AttributeValueAssertion,.
func (f FilterApproxMatch) size() int {
	return AttributeValueAssertion(f).sizeTagged(TagFilterApproxMatch)
}

func (f *FilterApproxMatch) AttributeDesc() AttributeDescription {
	return f.attributeDesc
}

func (f *FilterApproxMatch) AssertionValue() AssertionValue {
	return f.assertionValue
}
