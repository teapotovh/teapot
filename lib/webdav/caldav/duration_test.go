package caldav

import (
	"testing"
	"time"

	ics "github.com/arran4/golang-ical"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "time/tzdata"
)

func TestParseICalDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
		want  icalDuration
	}{
		{
			name:  "empty duration",
			value: "P",
			want:  icalDuration{},
		},
		{
			name:  "zero seconds",
			value: "PT0S",
			want:  icalDuration{},
		},
		{
			name:  "one day",
			value: "P1D",
			want:  icalDuration{days: 1},
		},
		{
			name:  "seven weeks",
			value: "P7W",
			want:  icalDuration{days: 49},
		},
		{
			name:  "two weeks",
			value: "P2W",
			want:  icalDuration{days: 14},
		},
		{
			name:  "one hour",
			value: "PT1H",
			want:  icalDuration{exact: time.Hour},
		},
		{
			name:  "thirty minutes",
			value: "PT30M",
			want:  icalDuration{exact: 30 * time.Minute},
		},
		{
			name:  "forty five seconds",
			value: "PT45S",
			want:  icalDuration{exact: 45 * time.Second},
		},
		{
			name:  "days and time",
			value: "P1DT2H3M4S",
			want: icalDuration{
				days:  1,
				exact: 2*time.Hour + 3*time.Minute + 4*time.Second,
			},
		},
		{
			name:  "RFC 5545 example",
			value: "P15DT5H0M20S",
			want: icalDuration{
				days:  15,
				exact: 5*time.Hour + 20*time.Second,
			},
		},
		{
			name:  "two days and time",
			value: "P2DT12H30M",
			want: icalDuration{
				days:  2,
				exact: 12*time.Hour + 30*time.Minute,
			},
		},
		{
			name:  "explicit positive sign",
			value: "+P1D",
			want:  icalDuration{days: 1},
		},
		{
			name:  "lowercase",
			value: "p1dt2h",
			want: icalDuration{
				days:  1,
				exact: 2 * time.Hour,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseICalDuration(tt.value)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseICalDurationNegative(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
		want  icalDuration
	}{
		{
			name:  "negative day",
			value: "-P1D",
			want:  icalDuration{days: -1},
		},
		{
			name:  "negative time",
			value: "-PT1H30M",
			want:  icalDuration{exact: -1*time.Hour - 30*time.Minute},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseICalDuration(tt.value)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.True(t, got.days < 0 || got.exact < 0)
		})
	}
}

func TestParseICalDurationInvalid(t *testing.T) {
	t.Parallel()

	tests := []string{
		"",
		"1D",
		"P1",
		"P1H",
		"PT1D",
		"P1S",
		"PT",
		"P1DT",
		"PT1H2D",
		"P-1D",
		"P1W2D",
		"P1.5D",
	}

	for _, tt := range tests {
		t.Run(tt, func(t *testing.T) {
			t.Parallel()

			_, err := parseICalDuration(tt)
			require.ErrorIs(t, err, ErrInvalidDuration)
		})
	}
}

func TestEndFromDurationNegative(t *testing.T) {
	t.Parallel()

	comp := ics.NewEvent("negative@example.com")
	comp.SetProperty(ics.ComponentPropertyDuration, "-P1D")

	_, err := endFromDuration(&comp.ComponentBase, time.Date(2006, time.January, 4, 10, 0, 0, 0, time.UTC))
	require.ErrorIs(t, err, ErrInvalidDuration)
}

func TestICalDurationAddToAcrossDST(t *testing.T) {
	t.Parallel()

	loc, err := time.LoadLocation("US/Eastern")
	require.NoError(t, err)

	start := time.Date(2006, time.April, 1, 12, 0, 0, 0, loc)
	want := time.Date(2006, time.April, 2, 12, 0, 0, 0, loc)

	assert.True(t, icalDuration{days: 1}.addTo(start).Equal(want))
}
