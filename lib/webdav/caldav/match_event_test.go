package caldav

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "embed"
)

//go:embed testdata/teapot-daily-rrule.ics
var teapotDailyRrule string

//go:embed testdata/teapot-event-duration-dst.ics
var teapotEventDurationDST string

//go:embed testdata/teapot-event-zero-length.ics
var teapotEventZeroLength string

//go:embed testdata/teapot-exdate.ics
var teapotExdate string

//go:embed testdata/teapot-invalid-duration.ics
var teapotInvalidDuration string

//go:embed testdata/teapot-multiple-exdate.ics
var teapotMultipleExdate string

//go:embed testdata/teapot-overlapping.ics
var teapotOverlapping string

//go:embed testdata/teapot-rdate.ics
var teapotRdate string

//go:embed testdata/teapot-zero-duration.ics
var teapotZeroDuration string

func TestMatchCalendarTimeRangeBoundaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		data  string
		start string
		end   string
		want  bool
	}{
		{
			name:  "range covers event",
			data:  teapotOverlapping,
			start: testTime0900,
			end:   testTime1200,
			want:  true,
		},
		{
			name:  "range start equals event start",
			data:  teapotOverlapping,
			start: testTime1000,
			end:   testTime1200,
			want:  true,
		},
		{
			name:  "range start equals event end",
			data:  teapotOverlapping,
			start: testTime1100,
			end:   testTime1200,
			want:  false,
		},
		{
			name:  "range end equals event start",
			data:  teapotOverlapping,
			start: testTime0900,
			end:   testTime1000,
			want:  false,
		},
		{
			name:  "range end equals event end",
			data:  teapotOverlapping,
			start: "20060104T103000Z",
			end:   testTime1100,
			want:  true,
		},
		{
			name:  "open-ended range overlaps event",
			data:  teapotOverlapping,
			start: "20060104T103000Z",
			end:   "",
			want:  true,
		},
		{
			name:  "event before open-ended range",
			data:  teapotOverlapping,
			start: testTime1200,
			end:   "",
			want:  false,
		},
		{
			name:  "zero-duration event at range start",
			data:  teapotZeroDuration,
			start: testTime1000,
			end:   testTime1100,
			want:  true,
		},
		{
			name:  "zero-duration event inside range",
			data:  teapotZeroDuration,
			start: testTime0900,
			end:   testTime1100,
			want:  true,
		},
		{
			name:  "zero-duration event at range end",
			data:  teapotZeroDuration,
			start: testTime0900,
			end:   testTime1000,
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cal := mustParseICS(t, tt.data)
			match, err := MatchCalendar(timeRangeQuery(t, tt.start, tt.end), cal)
			require.NoError(t, err)
			assert.Equal(t, tt.want, match)
		})
	}
}

func TestMatchCalendarInvalidDuration(t *testing.T) {
	t.Parallel()

	cal := mustParseICS(t, teapotInvalidDuration)

	_, err := MatchCalendar(timeRangeQuery(t, "20060104T000000Z", "20060105T000000Z"), cal)
	require.ErrorIs(t, err, ErrInvalidDuration)
}

func TestMatchCalendarNominalDurationAcrossDST(t *testing.T) {
	t.Parallel()

	cal := mustParseICS(t, teapotEventDurationDST)

	loc, err := time.LoadLocation("US/Eastern")
	require.NoError(t, err)

	at := func(hour, minute int) time.Time {
		return time.Date(2006, time.April, 2, hour, minute, 0, 0, loc)
	}

	query := func(start, end time.Time) CompFilter {
		return CompFilter{
			Name: testCalendarComponent,
			Comps: []CompFilter{{
				Name:  testEventComponent,
				Start: start,
				End:   end,
			}},
		}
	}

	tests := []struct {
		name  string
		start time.Time
		end   time.Time
		want  bool
	}{
		{
			name:  "range after nominal end",
			start: at(12, 30),
			end:   at(13, 30),
			want:  false,
		},
		{
			name:  "range before nominal end",
			start: at(11, 30),
			end:   at(12, 30),
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			match, err := MatchCalendar(query(tt.start, tt.end), cal)
			require.NoError(t, err)
			assert.Equal(t, tt.want, match)
		})
	}
}

func TestMatchCalendarExplicitZeroLength(t *testing.T) {
	t.Parallel()

	cal := mustParseICS(t, teapotEventZeroLength)

	tests := []struct {
		name  string
		start string
		end   string
		want  bool
	}{
		{
			name:  "range contains event instant",
			start: testTime0900,
			end:   testTime1100,
			want:  true,
		},
		{
			name:  "range starts at event instant",
			start: testTime1000,
			end:   testTime1100,
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			match, err := MatchCalendar(timeRangeQuery(t, tt.start, tt.end), cal)
			require.NoError(t, err)
			assert.Equal(t, tt.want, match)
		})
	}
}

func TestMatchCalendarRecurrence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		data  string
		start string
		end   string
		want  bool
	}{
		{
			name:  "recurring instance within range",
			data:  teapotDailyRrule,
			start: "20060104T000000Z",
			end:   "20060105T000000Z",
			want:  true,
		},
		{
			name:  "recurrence count exhausted",
			data:  teapotDailyRrule,
			start: "20060105T000000Z",
			end:   "20060106T000000Z",
			want:  false,
		},
		{
			name:  "excluded date does not match",
			data:  teapotExdate,
			start: "20060103T090000Z",
			end:   "20060103T120000Z",
			want:  false,
		},
		{
			name:  "excluded date keeps other instances",
			data:  teapotExdate,
			start: testTime0900,
			end:   testTime1200,
			want:  true,
		},
		{
			name:  "rdate instance within range",
			data:  teapotRdate,
			start: "20060110T090000Z",
			end:   "20060110T120000Z",
			want:  true,
		},
		{
			name:  "rdate instance outside range",
			data:  teapotRdate,
			start: "20060105T000000Z",
			end:   "20060106T000000Z",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cal := mustParseICS(t, tt.data)
			match, err := MatchCalendar(timeRangeQuery(t, tt.start, tt.end), cal)
			require.NoError(t, err)
			assert.Equal(t, tt.want, match)
		})
	}
}
