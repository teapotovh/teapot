package message

import (
	"errors"
)

//	SearchRequest ::= [APPLICATION 3] SEQUENCE {
//	     baseObject      LDAPDN,
//	     scope           ENUMERATED {
//	          baseObject              (0),
//	          singleLevel             (1),
//	          wholeSubtree            (2),
//	          ...  },
//	     derefAliases    ENUMERATED {
//	          neverDerefAliases       (0),
//	          derefInSearching        (1),
//	          derefFindingBaseObj     (2),
//	          derefAlways             (3) },
//	     sizeLimit       INTEGER (0 ..  maxInt),
//	     timeLimit       INTEGER (0 ..  maxInt),
//	     typesOnly       BOOLEAN,
//	     filter          Filter,
//	     attributes      AttributeSelection }
func readSearchRequest(bytes *Bytes) (searchrequest SearchRequest, err error) {
	err = bytes.ReadSubBytes(classApplication, TagSearchRequest, searchrequest.readComponents)
	if err != nil {
		err = LdapError{"readSearchRequest:\n" + err.Error()}
		return
	}

	return
}

func (s *SearchRequest) readComponents(bytes *Bytes) (err error) {
	s.baseObject, err = readLDAPDN(bytes)
	if err != nil {
		err = LdapError{"readComponents:\n" + err.Error()}
		return err
	}

	s.scope, err = readENUMERATED(bytes, EnumeratedSearchRequestScope)
	if err != nil {
		err = LdapError{"readComponents:\n" + err.Error()}
		return err
	}

	s.derefAliases, err = readENUMERATED(bytes, EnumeratedSearchRequestDerefAliases)
	if err != nil {
		err = LdapError{"readComponents:\n" + err.Error()}
		return err
	}

	s.sizeLimit, err = readPositiveINTEGER(bytes)
	if err != nil {
		err = LdapError{"readComponents:\n" + err.Error()}
		return err
	}

	s.timeLimit, err = readPositiveINTEGER(bytes)
	if err != nil {
		err = LdapError{"readComponents:\n" + err.Error()}
		return err
	}

	s.typesOnly, err = readBOOLEAN(bytes)
	if err != nil {
		err = LdapError{"readComponents:\n" + err.Error()}
		return err
	}

	s.filter, err = readFilter(bytes)
	if err != nil {
		err = LdapError{"readComponents:\n" + err.Error()}
		return err
	}

	s.attributes, err = readAttributeSelection(bytes)
	if err != nil {
		err = LdapError{"readComponents:\n" + err.Error()}
		return err
	}

	return err
}

//	SearchRequest ::= [APPLICATION 3] SEQUENCE {
//	     baseObject      LDAPDN,
//	     scope           ENUMERATED {
//	          baseObject              (0),
//	          singleLevel             (1),
//	          wholeSubtree            (2),
//	          ...  },
//	     derefAliases    ENUMERATED {
//	          neverDerefAliases       (0),
//	          derefInSearching        (1),
//	          derefFindingBaseObj     (2),
//	          derefAlways             (3) },
//	     sizeLimit       INTEGER (0 ..  maxInt),
//	     timeLimit       INTEGER (0 ..  maxInt),
//	     typesOnly       BOOLEAN,
//	     filter          Filter,
//	     attributes      AttributeSelection }
func (s SearchRequest) write(bytes *Bytes) (size int) {
	size += s.attributes.write(bytes)
	size += s.filter.write(bytes)
	size += s.typesOnly.write(bytes)
	size += s.timeLimit.write(bytes)
	size += s.sizeLimit.write(bytes)
	size += s.derefAliases.write(bytes)
	size += s.scope.write(bytes)
	size += s.baseObject.write(bytes)
	size += bytes.WriteTagAndLength(classApplication, isCompound, TagSearchRequest, size)

	return
}

//	SearchRequest ::= [APPLICATION 3] SEQUENCE {
//	     baseObject      LDAPDN,
//	     scope           ENUMERATED {
//	          baseObject              (0),
//	          singleLevel             (1),
//	          wholeSubtree            (2),
//	          ...  },
//	     derefAliases    ENUMERATED {
//	          neverDerefAliases       (0),
//	          derefInSearching        (1),
//	          derefFindingBaseObj     (2),
//	          derefAlways             (3) },
//	     sizeLimit       INTEGER (0 ..  maxInt),
//	     timeLimit       INTEGER (0 ..  maxInt),
//	     typesOnly       BOOLEAN,
//	     filter          Filter,
//	     attributes      AttributeSelection }
func (s SearchRequest) size() (size int) {
	size += s.baseObject.size()
	size += s.scope.size()
	size += s.derefAliases.size()
	size += s.sizeLimit.size()
	size += s.timeLimit.size()
	size += s.typesOnly.size()
	size += s.filter.size()
	size += s.attributes.size()
	size += sizeTagAndLength(TagSearchRequest, size)

	return
}

func (s *SearchRequest) BaseObject() LDAPDN {
	return s.baseObject
}

func (s *SearchRequest) Scope() ENUMERATED {
	return s.scope
}

func (s *SearchRequest) DerefAliases() ENUMERATED {
	return s.derefAliases
}

func (s *SearchRequest) SizeLimit() INTEGER {
	return s.sizeLimit
}

func (s *SearchRequest) TimeLimit() INTEGER {
	return s.timeLimit
}

func (s *SearchRequest) TypesOnly() BOOLEAN {
	return s.typesOnly
}

func (s *SearchRequest) Attributes() AttributeSelection {
	return s.attributes
}

func (s *SearchRequest) Filter() Filter {
	return s.filter
}

func (s *SearchRequest) FilterString() string {
	str, _ := s.decompileFilter(s.Filter())
	return str
}

//nolint:gocyclo
func (s *SearchRequest) decompileFilter(packet Filter) (ret string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New("error decompiling filter")
		}
	}()

	ret = "("
	err = nil

	var childStr string

	switch f := packet.(type) {
	case FilterAnd:
		ret += "&"

		for _, child := range f {
			childStr, err = s.decompileFilter(child)
			if err != nil {
				return ret, err
			}

			ret += childStr
		}
	case FilterOr:
		ret += "|"

		for _, child := range f {
			childStr, err = s.decompileFilter(child)
			if err != nil {
				return ret, err
			}

			ret += childStr
		}
	case FilterNot:
		ret += "!"

		childStr, err = s.decompileFilter(f.Filter)
		if err != nil {
			return ret, err
		}

		ret += childStr

	case FilterSubstrings:
		ret += string(f.Type_())
		ret += "="

		for _, fs := range f.Substrings() {
			switch fsv := fs.(type) {
			case SubstringInitial:
				ret += string(fsv) + "*"
			case SubstringAny:
				ret += "*" + string(fsv) + "*"
			case SubstringFinal:
				ret += "*" + string(fsv)
			}
		}
	case FilterEqualityMatch:
		ret += string(f.AttributeDesc())
		ret += "="
		ret += string(f.AssertionValue())
	case FilterGreaterOrEqual:
		ret += string(f.AttributeDesc())
		ret += ">="
		ret += string(f.AssertionValue())
	case FilterLessOrEqual:
		ret += string(f.AttributeDesc())
		ret += "<="
		ret += string(f.AssertionValue())
	case FilterPresent:
		// if 0 == len(packet.Children) {
		// 	ret += ber.DecodeString(packet.Data.Bytes())
		// } else {
		// 	ret += ber.DecodeString(packet.Children[0].Data.Bytes())
		// }
		ret += string(f)
		ret += "=*"
	case FilterApproxMatch:
		ret += string(f.AttributeDesc())
		ret += "~="
		ret += string(f.AssertionValue())
	}

	ret += ")"

	return ret, err
}
