package store

import (
	"encoding/json"
	"encoding/xml"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeadPropertiesJSONRoundTrip(t *testing.T) {
	t.Parallel()

	name := xml.Name{Space: "DAV:", Local: "details"}
	properties := DeadProperties{
		name: `<details xmlns="DAV:">My Name</details>`,
	}

	data, err := json.Marshal(properties)
	require.NoError(t, err)
	require.JSONEq(
		t,
		`[{"name":{"Space":"DAV:","Local":"details"},"value":"<details xmlns=\"DAV:\">My Name</details>"}]`,
		string(data),
	)

	var got DeadProperties
	require.NoError(t, json.Unmarshal(data, &got))
	require.Equal(t, properties, got)
}

func TestDeadPropertiesUnmarshalKeepsUniqueKeys(t *testing.T) {
	t.Parallel()

	name := xml.Name{Space: "DAV:", Local: "details"}
	data := `[` +
		`{"name":{"Space":"DAV:","Local":"details"},"value":"first"},` +
		`{"name":{"Space":"DAV:","Local":"details"},"value":"second"}` +
		`]`

	var properties DeadProperties
	require.NoError(t, json.Unmarshal([]byte(data), &properties))
	require.Equal(t, DeadProperties{name: "second"}, properties)
}
