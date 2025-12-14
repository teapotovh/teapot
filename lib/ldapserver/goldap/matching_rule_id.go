package message

// MatchingRuleId ::= LDAPString.
func readTaggedMatchingRuleID(bytes *Bytes, class int, tag int) (matchingruleid MatchingRuleID, err error) {
	var ldapstring LDAPString

	ldapstring, err = readTaggedLDAPString(bytes, class, tag)
	if err != nil {
		err = LdapError{"readTaggedMatchingRuleId:\n" + err.Error()}
		return
	}

	matchingruleid = MatchingRuleID(ldapstring)

	return
}
func (m MatchingRuleID) Pointer() *MatchingRuleID { return &m }

// MatchingRuleId ::= LDAPString.
func (m MatchingRuleID) writeTagged(bytes *Bytes, class int, tag int) int {
	return LDAPString(m).writeTagged(bytes, class, tag)
}

// MatchingRuleId ::= LDAPString.
func (m MatchingRuleID) sizeTagged(tag int) int {
	return LDAPString(m).sizeTagged(tag)
}
