package caldav

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "time/tzdata"
)

func TestParseTimeValue(t *testing.T) {
	t.Parallel()

	eastern, err := time.LoadLocation("US/Eastern")
	require.NoError(t, err)

	tests := []struct {
		name            string
		value           string
		params          map[string][]string
		defaultLocation *time.Location
		want            time.Time
	}{
		{
			name:            "date",
			value:           "20060104",
			params:          map[string][]string{"VALUE": {"DATE"}},
			defaultLocation: eastern,
			want:            time.Date(2006, time.January, 4, 0, 0, 0, 0, eastern),
		},
		{
			name:            "floating date-time",
			value:           "20060104T100000",
			defaultLocation: eastern,
			want:            time.Date(2006, time.January, 4, 10, 0, 0, 0, eastern),
		},
		{
			name:            "utc date-time",
			value:           "20060104T150000Z",
			defaultLocation: eastern,
			want:            time.Date(2006, time.January, 4, 15, 0, 0, 0, time.UTC),
		},
		{
			name:            "tzid date-time",
			value:           "20060104T100000",
			params:          map[string][]string{"TZID": {"US/Eastern"}},
			defaultLocation: time.UTC,
			want:            time.Date(2006, time.January, 4, 10, 0, 0, 0, eastern),
		},
		{
			name:            "quoted tzid date-time",
			value:           "20060104T100000",
			params:          map[string][]string{"TZID": {`"US/Eastern"`}},
			defaultLocation: time.UTC,
			want:            time.Date(2006, time.January, 4, 10, 0, 0, 0, eastern),
		},
		{
			name:            "nil default location",
			value:           "20060104T100000",
			defaultLocation: nil,
			want:            time.Date(2006, time.January, 4, 10, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseTimeValue(tt.value, tt.params, tt.defaultLocation)
			require.NoError(t, err)
			assert.Equal(t, tt.want.Format(time.RFC3339), got.Format(time.RFC3339))
		})
	}
}
