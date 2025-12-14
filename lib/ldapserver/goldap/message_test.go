package message

import (
	"fmt"
	"testing"
)

func toHex(b []byte) (r string) {
	r = "[ "
	for _, e := range b {
		r += fmt.Sprintf("0x%x ", e)
	}
	return r + "]"
}

func TestMessageID(t *testing.T) {
	m := NewLDAPMessageWithProtocolOp(UnbindRequest{})
	m.SetMessageID(128)
	buf, err := m.Write()
	if err != nil {
		t.Errorf("marshalling failed with %v", err)
	}
	t.Logf("%v", toHex(buf.Bytes()))

	ret, err := ReadLDAPMessage(NewBytes(0, buf.Bytes()))
	if err != nil {
		t.Errorf("unmarshalling failed with %v", err)
	}
	if _, ok := ret.ProtocolOp().(UnbindRequest); !ok {
		t.Errorf("should be an unbind request")
	}
	if ret.MessageID() != 128 {
		t.Errorf("Expect message id 128, got %d", ret.MessageID())
	}
	t.Log("Done, marshal/unmarshall worked")
}

func TestSearchEntry(t *testing.T) {
	m := NewLDAPMessageWithProtocolOp(SearchResultEntry{
		objectName: "cn=êige€nbgtz,ou=users,dc=deuxfleurs,dc=fr",
		attributes: PartialAttributeList{
			PartialAttribute{
				type_: "displayname",
				vals:  []AttributeValue{"êiGe€NBgTZ"},
			},
			PartialAttribute{
				type_: "objectclass",
				vals:  []AttributeValue{"inetOrgPerson"},
			},
			PartialAttribute{
				type_: "objectclass",
				vals:  []AttributeValue{"organizationalPerson"},
			},
			PartialAttribute{
				type_: "objectclass",
				vals:  []AttributeValue{"person"},
			},
			PartialAttribute{
				type_: "objectclass",
				vals:  []AttributeValue{"top"},
			},
			PartialAttribute{
				type_: "structuralobjectclass",
				vals:  []AttributeValue{"inetOrgPerson"},
			},
		},
	})
	m.SetMessageID(24)
	buf, err := m.Write()
	if err != nil {
		t.Errorf("marshalling failed with %v", err)
	}
	if buf.Bytes()[0] != 0x30 {
		t.Logf("Malformed message: %v", toHex(buf.Bytes()))
	}
}
