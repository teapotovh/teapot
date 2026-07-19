package ldap

import ber "github.com/go-asn1-ber/asn1-ber"

type ControlTraceContext struct {
	Traceparent string
	Tracestate  string
}

func (c *ControlTraceContext) GetControlType() string {
	return "1.3.6.1.4.1.1337.1"
}

func (c *ControlTraceContext) String() string {
	return "Trace Context"
}

func (c *ControlTraceContext) Encode() *ber.Packet {
	payload := c.Traceparent + "\x00" + c.Tracestate

	control := ber.Encode(
		ber.ClassUniversal,
		ber.TypeConstructed,
		ber.TagSequence,
		nil,
		"Control",
	)

	// controlType LDAPOID
	control.AppendChild(
		ber.NewString(
			ber.ClassUniversal,
			ber.TypePrimitive,
			ber.TagOctetString,
			c.GetControlType(),
			"Control Type",
		),
	)

	// controlValue OCTET STRING
	control.AppendChild(
		ber.NewString(
			ber.ClassUniversal,
			ber.TypePrimitive,
			ber.TagOctetString,
			payload,
			"Control Value",
		),
	)

	return control
}
