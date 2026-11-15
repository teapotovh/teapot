package store

import (
	"encoding/json"
	"encoding/xml"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCalendarPropertiesJSONRoundTrip(t *testing.T) {
	t.Parallel()

	name := xml.Name{Space: "DAV:", Local: "displayname"}
	properties := CalendarProperties{
		name: `<displayname xmlns="DAV:">Work</displayname>`,
	}

	data, err := json.Marshal(CalendarMetadata{Properties: properties})
	require.NoError(t, err)
	require.JSONEq(
		t,
		`{"properties":[{"name":{"Space":"DAV:","Local":"displayname"},"value":"<displayname xmlns=\"DAV:\">Work</displayname>"}]}`,
		string(data),
	)

	var got CalendarMetadata
	require.NoError(t, json.Unmarshal(data, &got))
	require.Equal(t, properties, got.Properties)
}

func TestCalendarPropertiesUnmarshalKeepsUniqueKeys(t *testing.T) {
	t.Parallel()

	name := xml.Name{Space: "DAV:", Local: "details"}
	data := `{"properties":[` +
		`{"name":{"Space":"DAV:","Local":"details"},"value":"first"},` +
		`{"name":{"Space":"DAV:","Local":"details"},"value":"second"}` +
		`]}`

	var metadata CalendarMetadata
	require.NoError(t, json.Unmarshal([]byte(data), &metadata))
	require.Equal(t, CalendarProperties{name: "second"}, metadata.Properties)
}

func TestCalendarCloneCopiesProperties(t *testing.T) {
	t.Parallel()

	name := xml.Name{Space: "DAV:", Local: "displayname"}
	calendar := Calendar{
		Path: "/calendars/alice/work/",
		Metadata: CalendarMetadata{
			Properties: CalendarProperties{
				name: `<displayname xmlns="DAV:">Work</displayname>`,
			},
		},
	}

	cloned := calendar.Clone()
	cloned.Metadata.Properties[name] = `<displayname xmlns="DAV:">Other</displayname>`

	require.Equal(t, `<displayname xmlns="DAV:">Work</displayname>`, calendar.Metadata.Properties[name])
	require.Equal(t, `<displayname xmlns="DAV:">Other</displayname>`, cloned.Metadata.Properties[name])
}

func TestCalendarCloneKeepsNilProperties(t *testing.T) {
	t.Parallel()

	calendar := Calendar{Path: "/calendars/alice/work/"}

	cloned := calendar.Clone()
	require.Nil(t, cloned.Metadata.Properties)
}
