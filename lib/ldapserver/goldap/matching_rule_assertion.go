package message

//	MatchingRuleAssertion ::= SEQUENCE {
//	     matchingRule    [1] MatchingRuleId OPTIONAL,
//	     type            [2] AttributeDescription OPTIONAL,
//	     matchValue      [3] AssertionValue,
//	     dnAttributes    [4] BOOLEAN DEFAULT FALSE }
func readTaggedMatchingRuleAssertion(bytes *Bytes, class int, tag int) (ret MatchingRuleAssertion, err error) {
	err = bytes.ReadSubBytes(class, tag, ret.readComponents)
	if err != nil {
		err = LdapError{"readTaggedMatchingRuleAssertion:\n" + err.Error()}
		return
	}
	return
}

func (m *MatchingRuleAssertion) readComponents(bytes *Bytes) (err error) {
	err = m.readMatchingRule(bytes)
	if err != nil {
		return LdapError{"readComponents: " + err.Error()}
	}
	err = m.readType(bytes)
	if err != nil {
		return LdapError{"readComponents: " + err.Error()}
	}
	m.matchValue, err = readTaggedAssertionValue(
		bytes,
		classContextSpecific,
		TagMatchingRuleAssertionMatchValue,
	)
	if err != nil {
		return LdapError{"readComponents: " + err.Error()}
	}
	m.dnAttributes, err = readTaggedBOOLEAN(
		bytes,
		classContextSpecific,
		TagMatchingRuleAssertionDnAttributes,
	)
	if err != nil {
		return LdapError{"readComponents: " + err.Error()}
	}
	return
}

func (m *MatchingRuleAssertion) readMatchingRule(bytes *Bytes) (err error) {
	var tagAndLength TagAndLength
	tagAndLength, err = bytes.PreviewTagAndLength()
	if err != nil {
		return LdapError{"readMatchingRule: " + err.Error()}
	}
	if tagAndLength.Tag == TagMatchingRuleAssertionMatchingRule {
		var matchingRule MatchingRuleID
		matchingRule, err = readTaggedMatchingRuleID(bytes, classContextSpecific, TagMatchingRuleAssertionMatchingRule)
		if err != nil {
			return LdapError{"readMatchingRule: " + err.Error()}
		}
		m.matchingRule = matchingRule.Pointer()
	}
	return
}

func (m *MatchingRuleAssertion) readType(bytes *Bytes) (err error) {
	var tagAndLength TagAndLength
	tagAndLength, err = bytes.PreviewTagAndLength()
	if err != nil {
		return LdapError{"readType: " + err.Error()}
	}
	if tagAndLength.Tag == TagMatchingRuleAssertionType {
		var attributedescription AttributeDescription
		attributedescription, err = readTaggedAttributeDescription(
			bytes,
			classContextSpecific,
			TagMatchingRuleAssertionType,
		)
		if err != nil {
			return LdapError{"readType: " + err.Error()}
		}
		m.type_ = &attributedescription
	}
	return
}

func (m MatchingRuleAssertion) writeTagged(bytes *Bytes, class int, tag int) (size int) {
	if m.dnAttributes != BOOLEAN(false) {
		size += m.dnAttributes.writeTagged(bytes, classContextSpecific, TagMatchingRuleAssertionDnAttributes)
	}
	size += m.matchValue.writeTagged(bytes, classContextSpecific, TagMatchingRuleAssertionMatchValue)
	if m.type_ != nil {
		size += m.type_.writeTagged(bytes, classContextSpecific, TagMatchingRuleAssertionType)
	}
	if m.matchingRule != nil {
		size += m.matchingRule.writeTagged(bytes, classContextSpecific, TagMatchingRuleAssertionMatchingRule)
	}
	size += bytes.WriteTagAndLength(class, isCompound, tag, size)
	return
}

//	MatchingRuleAssertion ::= SEQUENCE {
//	     matchingRule    [1] MatchingRuleId OPTIONAL,
//	     type            [2] AttributeDescription OPTIONAL,
//	     matchValue      [3] AssertionValue,
//	     dnAttributes    [4] BOOLEAN DEFAULT FALSE }
func (m MatchingRuleAssertion) write(bytes *Bytes) (size int) {
	return m.writeTagged(bytes, classUniversal, tagSequence)
}

//	MatchingRuleAssertion ::= SEQUENCE {
//	     matchingRule    [1] MatchingRuleId OPTIONAL,
//	     type            [2] AttributeDescription OPTIONAL,
//	     matchValue      [3] AssertionValue,
//	     dnAttributes    [4] BOOLEAN DEFAULT FALSE }
func (m MatchingRuleAssertion) size() (size int) {
	return m.sizeTagged(tagSequence)
}

func (m MatchingRuleAssertion) sizeTagged(tag int) (size int) {
	if m.matchingRule != nil {
		size += m.matchingRule.sizeTagged(TagMatchingRuleAssertionMatchingRule)
	}
	if m.type_ != nil {
		size += m.type_.sizeTagged(TagMatchingRuleAssertionType)
	}
	size += m.matchValue.sizeTagged(TagMatchingRuleAssertionMatchValue)
	if m.dnAttributes != BOOLEAN(false) {
		size += m.dnAttributes.sizeTagged(TagMatchingRuleAssertionDnAttributes)
	}
	size += sizeTagAndLength(tag, size)
	return
}
