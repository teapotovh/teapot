package message

import "fmt"

//
//        AuthenticationChoice ::= CHOICE {
//             simple                  [0] OCTET STRING,
//                                     -- 1 and 2 reserved
//             sasl                    [3] SaslCredentials,
//             ...  }

func readAuthenticationChoice(bytes *Bytes) (ret AuthenticationChoice, err error) {
	tagAndLength, err := bytes.PreviewTagAndLength()
	if err != nil {
		err = LdapError{"readAuthenticationChoice:\n" + err.Error()}
		return ret, err
	}

	err = tagAndLength.ExpectClass(classContextSpecific)
	if err != nil {
		err = LdapError{"readAuthenticationChoice:\n" + err.Error()}
		return ret, err
	}

	switch tagAndLength.Tag {
	case TagAuthenticationChoiceSimple:
		ret, err = readTaggedOCTETSTRING(bytes, classContextSpecific, TagAuthenticationChoiceSimple)
	case TagAuthenticationChoiceSaslCredentials:
		ret, err = readSaslCredentials(bytes)
	default:
		err = LdapError{
			fmt.Sprintf("readAuthenticationChoice: invalid tag value %d for AuthenticationChoice", tagAndLength.Tag),
		}

		return ret, err
	}

	if err != nil {
		err = LdapError{"readAuthenticationChoice:\n" + err.Error()}
		return ret, err
	}

	return ret, err
}
