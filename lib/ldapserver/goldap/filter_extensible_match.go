package message

// extensibleMatch [9] MatchingRuleAssertion,.
func readFilterExtensibleMatch(bytes *Bytes) (filterextensiblematch FilterExtensibleMatch, err error) {
	var matchingruleassertion MatchingRuleAssertion

	matchingruleassertion, err = readTaggedMatchingRuleAssertion(bytes, classContextSpecific, TagFilterExtensibleMatch)
	if err != nil {
		err = LdapError{"readFilterExtensibleMatch:\n" + err.Error()}
		return
	}

	filterextensiblematch = FilterExtensibleMatch(matchingruleassertion)

	return
}

// extensibleMatch [9] MatchingRuleAssertion,.
func (f FilterExtensibleMatch) write(bytes *Bytes) int {
	return MatchingRuleAssertion(f).writeTagged(bytes, classContextSpecific, TagFilterExtensibleMatch)
}

func (f FilterExtensibleMatch) getFilterTag() int {
	return TagFilterExtensibleMatch
}

// extensibleMatch [9] MatchingRuleAssertion,.
func (f FilterExtensibleMatch) size() int {
	return MatchingRuleAssertion(f).sizeTagged(TagFilterExtensibleMatch)
}
