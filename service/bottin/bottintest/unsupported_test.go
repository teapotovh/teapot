package bottintest

import (
	"testing"
)

// Adapted from the Bauble assertions for features Bottin does not implement.
//
// Modify DN:
// https://github.com/rumba-id/bauble/blob/ce079b46a8996ea0b8a1ec5bd07def580944579b/src/bauble/suites/rfc4511/moddn.py
//
// Controls:
// https://github.com/rumba-id/bauble/blob/ce079b46a8996ea0b8a1ec5bd07def580944579b/src/bauble/suites/rfc4511/controls.py
//
// Who Am I:
// https://github.com/rumba-id/bauble/blob/ce079b46a8996ea0b8a1ec5bd07def580944579b/src/bauble/suites/extended/rfc4532.py
//
// Paged results:
// https://github.com/rumba-id/bauble/blob/ce079b46a8996ea0b8a1ec5bd07def580944579b/src/bauble/suites/controls/rfc2696.py
//
// Malformed PDU:
// https://github.com/rumba-id/bauble/blob/ce079b46a8996ea0b8a1ec5bd07def580944579b/src/bauble/suites/rfc4511/bind.py
func TestBaubleUnsupportedFeatures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		skip string
	}{
		{name: "modify dn", skip: skipModifyDN},
		{name: "controls", skip: skipControls},
		{name: "who am i", skip: skipWhoAmI},
		{name: "paged results", skip: skipPagedResults},
		{name: "malformed pdu", skip: skipMalformedPDU},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			t.Skip(test.skip)
		})
	}
}
