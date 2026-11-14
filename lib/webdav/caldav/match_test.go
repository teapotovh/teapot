package caldav

import (
	"strings"
	"testing"
	"time"

	ics "github.com/arran4/golang-ical"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "embed"
	_ "time/tzdata"
)

//go:embed testdata/rfc4791-event-attendees.ics
var rfc4791EventAttendees string

//go:embed testdata/rfc4791-event-recurring.ics
var rfc4791EventRecurring string

//go:embed testdata/rfc4791-event-single.ics
var rfc4791EventSingle string

//go:embed testdata/rfc4791-freebusy.ics
var rfc4791Freebusy string

//go:embed testdata/rfc4791-todo-cancelled.ics
var rfc4791TodoCancelled string

//go:embed testdata/rfc4791-todo-completed.ics
var rfc4791TodoCompleted string

//go:embed testdata/rfc4791-todo-due-jan-4.ics
var rfc4791TodoDueJan4 string

//go:embed testdata/rfc4791-todo-due-jan-6.ics
var rfc4791TodoDueJan6 string

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

//go:embed testdata/teapot-overlapping.ics
var teapotOverlapping string

//go:embed testdata/teapot-rdate.ics
var teapotRdate string

//go:embed testdata/teapot-zero-duration.ics
var teapotZeroDuration string

const (
	testCalendarComponent = "VCALENDAR"
	testEventComponent    = "VEVENT"
	testTodoComponent     = "VTODO"
	testAlarmComponent    = "VALARM"

	attendeesPath = "/bernard/work/event-attendees.ics"
)

type appendixBFixture struct {
	name string
	data string
	uid  string
}

// path returns the calendar object path used by the RFC 4791 examples.
func (f appendixBFixture) path() string {
	return "/bernard/work/" + f.name
}

var appendixBFixtures = []appendixBFixture{
	{name: "event-single.ics", data: rfc4791EventSingle, uid: "74855313FA803DA593CD579A@example.com"},
	{name: "event-recurring.ics", data: rfc4791EventRecurring, uid: "00959BC664CA650E933C892C@example.com"},
	{name: "event-attendees.ics", data: rfc4791EventAttendees, uid: "DC6C50A017428C5216A2F1CD@example.com"},
	{name: "todo-due-jan-4.ics", data: rfc4791TodoDueJan4, uid: "DDDEEB7915FA61233B861457@example.com"},
	{name: "todo-due-jan-6.ics", data: rfc4791TodoDueJan6, uid: "E10BA47467C5C69BB74E8720@example.com"},
	{name: "todo-completed.ics", data: rfc4791TodoCompleted, uid: "E10BA47467C5C69BB74E8722@example.com"},
	{name: "todo-cancelled.ics", data: rfc4791TodoCancelled, uid: "E10BA47467C5C69BB74E8725@example.com"},
	{name: "freebusy.ics", data: rfc4791Freebusy, uid: "76ef34-54a3d2@example.com"},
}

func mustParseICS(t *testing.T, data string) *ics.Calendar {
	t.Helper()

	cal, err := ics.ParseCalendar(strings.NewReader(data))
	require.NoError(t, err)

	return cal
}

func firstComponentID(t *testing.T, cal *ics.Calendar) string {
	t.Helper()

	for _, comp := range cal.Components {
		switch c := comp.(type) {
		case *ics.VEvent:
			return c.Id()
		case *ics.VTodo:
			return c.Id()
		case *ics.VBusy:
			return c.Id()
		}
	}

	require.FailNow(t, "calendar has no identifiable component")

	return ""
}

func appendixBCalendarObjects(t *testing.T) []CalendarObject {
	t.Helper()

	cos := make([]CalendarObject, 0, len(appendixBFixtures))
	for _, fixture := range appendixBFixtures {
		cal := mustParseICS(t, fixture.data)
		require.Equal(t, fixture.uid, firstComponentID(t, cal), "fixture %s", fixture.name)

		cos = append(cos, CalendarObject{
			Path: fixture.path(),
			Data: cal,
		})
	}

	return cos
}

func mustICSTime(t *testing.T, value string) time.Time {
	t.Helper()

	parsed, err := time.Parse(dateWithUTCTimeLayout, value)
	require.NoError(t, err)

	return parsed
}

func optionalICSTime(t *testing.T, value string) time.Time {
	t.Helper()

	if value == "" {
		return time.Time{}
	}

	return mustICSTime(t, value)
}

func objectPaths(cos []CalendarObject) []string {
	paths := make([]string, 0, len(cos))
	for _, co := range cos {
		paths = append(paths, co.Path)
	}

	return paths
}

func timeRangeQuery(t *testing.T, start, end string) CompFilter {
	t.Helper()

	return CompFilter{
		Name: testCalendarComponent,
		Comps: []CompFilter{{
			Name:  testEventComponent,
			Start: optionalICSTime(t, start),
			End:   optionalICSTime(t, end),
		}},
	}
}

func TestFilterRFC4791Examples(t *testing.T) {
	t.Parallel()

	cos := appendixBCalendarObjects(t)

	tests := []struct {
		name  string
		query *CompFilter
		want  []string
	}{
		{
			name: "7.8.1 events matching a time range",
			query: &CompFilter{
				Name: testCalendarComponent,
				Comps: []CompFilter{{
					Name:  testEventComponent,
					Start: mustICSTime(t, "20060104T000000Z"),
					End:   mustICSTime(t, "20060105T000000Z"),
				}},
			},
			want: []string{"/bernard/work/event-recurring.ics", attendeesPath},
		},
		{
			name: "7.8.6 event by UID",
			query: &CompFilter{
				Name: testCalendarComponent,
				Comps: []CompFilter{{
					Name: testEventComponent,
					Props: []PropFilter{{
						Name:      "UID",
						TextMatch: &TextMatch{Text: "DC6C50A017428C5216A2F1CD@example.com"},
					}},
				}},
			},
			want: []string{attendeesPath},
		},
		{
			name: "7.8.7 events by PARTSTAT",
			query: &CompFilter{
				Name: testCalendarComponent,
				Comps: []CompFilter{{
					Name: testEventComponent,
					Props: []PropFilter{{
						Name:      "ATTENDEE",
						TextMatch: &TextMatch{Text: "mailto:lisa@example.com"},
						ParamFilter: []ParamFilter{{
							Name:      "PARTSTAT",
							TextMatch: &TextMatch{Text: "NEEDS-ACTION"},
						}},
					}},
				}},
			},
			want: []string{attendeesPath},
		},
		{
			name: "7.8.8 events only",
			query: &CompFilter{
				Name:  testCalendarComponent,
				Comps: []CompFilter{{Name: testEventComponent}},
			},
			want: []string{
				"/bernard/work/event-single.ics",
				"/bernard/work/event-recurring.ics",
				attendeesPath,
			},
		},
		{
			name: "non-standard property filter with no match",
			query: &CompFilter{
				Name: testCalendarComponent,
				Comps: []CompFilter{{
					Name: testEventComponent,
					Props: []PropFilter{{
						Name:      "X-ABC-GUID",
						TextMatch: &TextMatch{Text: "ABC"},
					}},
				}},
			},
			want: []string{},
		},

		{
			name: "negated text-match",
			query: &CompFilter{
				Name: testCalendarComponent,
				Comps: []CompFilter{{
					Name: testEventComponent,
					Props: []PropFilter{{
						Name: "UID",
						TextMatch: &TextMatch{
							Text:            "DC6C50A017428C5216A2F1CD@example.com",
							NegateCondition: true,
						},
					}},
				}},
			},
			want: []string{"/bernard/work/event-single.ics", "/bernard/work/event-recurring.ics"},
		},
		{
			name: "prop-filter is-not-defined excludes defined properties",
			query: &CompFilter{
				Name: testCalendarComponent,
				Comps: []CompFilter{{
					Name:  testEventComponent,
					Props: []PropFilter{{Name: "SUMMARY", IsNotDefined: true}},
				}},
			},
			want: []string{},
		},
		{
			name: "prop-filter is-not-defined matches missing properties",
			query: &CompFilter{
				Name: testCalendarComponent,
				Comps: []CompFilter{{
					Name:  testEventComponent,
					Props: []PropFilter{{Name: "LOCATION", IsNotDefined: true}},
				}},
			},
			want: []string{
				"/bernard/work/event-single.ics",
				"/bernard/work/event-recurring.ics",
				attendeesPath,
			},
		},
		{
			name: "comp-filter is-not-defined matches events without nested components",
			query: &CompFilter{
				Name: testCalendarComponent,
				Comps: []CompFilter{{
					Name:  testEventComponent,
					Comps: []CompFilter{{Name: testAlarmComponent, IsNotDefined: true}},
				}},
			},
			want: []string{
				"/bernard/work/event-single.ics",
				"/bernard/work/event-recurring.ics",
				attendeesPath,
			},
		},
		{
			name: "comp-filter without is-not-defined requires a nested component",
			query: &CompFilter{
				Name: testCalendarComponent,
				Comps: []CompFilter{{
					Name:  testEventComponent,
					Comps: []CompFilter{{Name: testAlarmComponent}},
				}},
			},
			want: []string{},
		},
		{
			name: "text-match on any attendee",
			query: &CompFilter{
				Name: testCalendarComponent,
				Comps: []CompFilter{{
					Name: testEventComponent,
					Props: []PropFilter{{
						Name:      "ATTENDEE",
						TextMatch: &TextMatch{Text: "mailto:cyrus@example.com"},
					}},
				}},
			},
			want: []string{attendeesPath},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := Filter(tt.query, cos)
			require.NoError(t, err)
			assert.Equal(t, tt.want, objectPaths(got))
		})
	}
}

func TestFilterNilQuery(t *testing.T) {
	t.Parallel()

	cos := appendixBCalendarObjects(t)

	got, err := Filter(nil, cos)
	require.NoError(t, err)
	assert.Equal(t, cos, got)
}

func TestFilterInvalidTopLevel(t *testing.T) {
	t.Parallel()

	cos := appendixBCalendarObjects(t)

	_, err := Filter(&CompFilter{Name: testEventComponent}, cos)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidTopLevel)
}

func TestMatchCalendarEmptyObject(t *testing.T) {
	t.Parallel()

	_, err := MatchCalendar(CompFilter{Name: testCalendarComponent}, nil)
	assert.ErrorIs(t, err, ErrMatchEmptyObject)

	_, err = MatchCalendar(CompFilter{Name: testCalendarComponent}, &ics.Calendar{})
	assert.ErrorIs(t, err, ErrMatchEmptyObject)
}

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
			start: "20060104T090000Z",
			end:   "20060104T120000Z",
			want:  true,
		},
		{
			name:  "range start equals event start",
			data:  teapotOverlapping,
			start: "20060104T100000Z",
			end:   "20060104T120000Z",
			want:  true,
		},
		{
			name:  "range start equals event end",
			data:  teapotOverlapping,
			start: "20060104T110000Z",
			end:   "20060104T120000Z",
			want:  false,
		},
		{
			name:  "range end equals event start",
			data:  teapotOverlapping,
			start: "20060104T090000Z",
			end:   "20060104T100000Z",
			want:  false,
		},
		{
			name:  "range end equals event end",
			data:  teapotOverlapping,
			start: "20060104T103000Z",
			end:   "20060104T110000Z",
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
			start: "20060104T120000Z",
			end:   "",
			want:  false,
		},
		{
			name:  "zero-duration event at range start",
			data:  teapotZeroDuration,
			start: "20060104T100000Z",
			end:   "20060104T110000Z",
			want:  true,
		},
		{
			name:  "zero-duration event inside range",
			data:  teapotZeroDuration,
			start: "20060104T090000Z",
			end:   "20060104T110000Z",
			want:  true,
		},
		{
			name:  "zero-duration event at range end",
			data:  teapotZeroDuration,
			start: "20060104T090000Z",
			end:   "20060104T100000Z",
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
			start: "20060104T090000Z",
			end:   "20060104T110000Z",
			want:  true,
		},
		{
			name:  "range starts at event instant",
			start: "20060104T100000Z",
			end:   "20060104T110000Z",
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
			start: "20060104T090000Z",
			end:   "20060104T120000Z",
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

func TestMatchCalendarVTodoTimeRange(t *testing.T) {
	t.Parallel()

	// RFC 4791, section 9.9 defines time-range matching for VTODO components,
	// but MatchCalendar only implements VEVENT filtering.
	t.Skip("TODO: VTODO time-range matching from RFC 4791 section 9.9 is not implemented")

	cos := appendixBCalendarObjects(t)

	query := &CompFilter{
		Name: testCalendarComponent,
		Comps: []CompFilter{{
			Name:  testTodoComponent,
			Start: mustICSTime(t, "20060101T000000Z"),
			End:   mustICSTime(t, "20060105T000000Z"),
		}},
	}

	got, err := Filter(query, cos)
	require.NoError(t, err)
	assert.Equal(t, []string{"/bernard/work/todo-due-jan-4.ics"}, objectPaths(got))
}

func TestMatchCalendarValarmTimeRange(t *testing.T) {
	t.Parallel()

	// RFC 4791, section 7.8.5 relies on VALARM time-range matching, but
	// matchCompFilter only descends into VEVENT components.
	t.Skip("TODO: VALARM time-range matching from RFC 4791 section 7.8.5 is not implemented")

	cos := appendixBCalendarObjects(t)

	query := &CompFilter{
		Name: testCalendarComponent,
		Comps: []CompFilter{{
			Name: testTodoComponent,
			Comps: []CompFilter{{
				Name:  testAlarmComponent,
				Start: mustICSTime(t, "20060104T000000Z"),
				End:   mustICSTime(t, "20060105T000000Z"),
			}},
		}},
	}

	got, err := Filter(query, cos)
	require.NoError(t, err)
	assert.Equal(t, []string{"/bernard/work/todo-due-jan-4.ics", "/bernard/work/todo-due-jan-6.ics"}, objectPaths(got))
}
