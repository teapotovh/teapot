package caldav

import (
	"encoding/xml"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/teapotovh/teapot/lib/webdav/internal"
)

func TestCalendarPropertiesAccessors(t *testing.T) {
	t.Parallel()

	var properties CalendarProperties

	name := properties.DisplayName()
	require.Empty(t, name)

	description := properties.Description()
	require.Empty(t, description)

	color := properties.Color()
	require.Empty(t, color)

	components, err := properties.SupportedComponentSet()
	require.NoError(t, err)
	require.Empty(t, components)

	size, err := properties.MaxResourceSize()
	require.NoError(t, err)
	require.Zero(t, size)

	require.NoError(t, properties.SetDisplayName("Work"))
	require.NoError(t, properties.SetDescription("Team calendar"))
	require.NoError(t, properties.SetColor("#FF0000"))
	require.NoError(t, properties.SetSupportedComponentSet([]string{"VEVENT", "VTODO"}))
	require.NoError(t, properties.SetMaxResourceSize(4096))

	name = properties.DisplayName()
	require.Equal(t, "Work", name)

	description = properties.Description()
	require.Equal(t, "Team calendar", description)

	color = properties.Color()
	require.Equal(t, "#FF0000", color)

	components, err = properties.SupportedComponentSet()
	require.NoError(t, err)
	require.Equal(t, []string{"VEVENT", "VTODO"}, components)

	size, err = properties.MaxResourceSize()
	require.NoError(t, err)
	require.EqualValues(t, 4096, size)

	properties.Set(xml.Name{Space: "urn:example", Local: "custom"}, "<custom/>")
	require.Equal(t, "<custom/>", properties.Get(xml.Name{Space: "urn:example", Local: "custom"}))
	properties.Remove(xml.Name{Space: "urn:example", Local: "custom"})
	require.Empty(t, properties.Get(xml.Name{Space: "urn:example", Local: "custom"}))
}

func TestCalendarPropertiesInvalidValues(t *testing.T) {
	t.Parallel()

	properties := NewCalendarProperties(map[xml.Name]string{
		internal.DisplayNameName: "<displayname>",
		maxResourceSizeName:      `<max-resource-size xmlns="urn:ietf:params:xml:ns:caldav">large</max-resource-size>`,
	})

	name := properties.DisplayName()
	require.Empty(t, name)

	size, err := properties.MaxResourceSize()
	require.Error(t, err)
	require.Zero(t, size)
}
