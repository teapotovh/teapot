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

//go:embed testdata/teapot-multiple-exdate.ics
var teapotMultipleExdate string

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

	testTime0800 = "20060104T080000Z"
	testTime0900 = "20060104T090000Z"
	testTime0930 = "20060104T093000Z"
	testTime1000 = "20060104T100000Z"
	testTime1001 = "20060104T100100Z"
	testTime1100 = "20060104T110000Z"
	testTime1200 = "20060104T120000Z"
	testTime1201 = "20060104T120100Z"
	testTime1300 = "20060104T130000Z"

	testTodoStartProp     = "DTSTART:20060104T100000Z"
	testTodoDue1200Prop   = "DUE:20060104T120000Z"
	testTodoDue1000Prop   = "DUE:20060104T100000Z"
	testTodoDurationProp  = "DURATION:PT2H"
	testTodoCompletedProp = "COMPLETED:20060104T120000Z"
	testTodoCreatedProp   = "CREATED:20060104T100000Z"

	testTodoCreated1200Prop   = "CREATED:20060104T120000Z"
	testTodoCompleted1000Prop = "COMPLETED:20060104T100000Z"
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

func TestMatchCalendarPropDateFilterAnyMatch(t *testing.T) {
	t.Parallel()

	cal := mustParseICS(t, teapotMultipleExdate)

	tests := []struct {
		name  string
		start string
		end   string
		want  bool
	}{
		{
			name:  "one excluded date in range",
			start: "20060103T000000Z",
			end:   "20060104T000000Z",
			want:  true,
		},
		{
			name:  "no excluded date in range",
			start: "20060106T000000Z",
			end:   "20060107T000000Z",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			query := &CompFilter{
				Name: testCalendarComponent,
				Comps: []CompFilter{{
					Name: testEventComponent,
					Props: []PropFilter{{
						Name:  "EXDATE",
						Start: mustICSTime(t, tt.start),
						End:   mustICSTime(t, tt.end),
					}},
				}},
			}

			match, err := MatchCalendar(*query, cal)
			require.NoError(t, err)
			assert.Equal(t, tt.want, match)
		})
	}
}

func TestMatchCalendarVTodoTimeRange(t *testing.T) {
	t.Parallel()

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

func todoCalendar(properties ...string) string {
	lines := []string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//Example Corp.//CalDAV Client//EN",
		"BEGIN:VTODO",
		"UID:todo@example.com",
		"DTSTAMP:20060101T000000Z",
	}

	lines = append(lines, properties...)
	lines = append(lines, "END:VTODO", "END:VCALENDAR")

	return strings.Join(lines, "\n")
}

func TestMatchCalendarVTodoTimeRangeStates(t *testing.T) {
	t.Parallel()

	// Each group covers a row of the VTODO table in RFC 4791, section 9.9.
	tests := []struct {
		name       string
		properties []string
		start      string
		end        string
		want       bool
	}{
		{
			name:       "dtstart and duration, range ends at dtstart",
			properties: []string{testTodoStartProp, testTodoDurationProp},
			start:      testTime0900,
			end:        testTime1000,
			want:       false,
		},
		{
			name:       "dtstart and duration, range starts after dtstart",
			properties: []string{testTodoStartProp, testTodoDurationProp},
			start:      testTime0900,
			end:        testTime1001,
			want:       true,
		},
		{
			name:       "dtstart and duration, range starts at effective end",
			properties: []string{testTodoStartProp, testTodoDurationProp},
			start:      testTime1200,
			end:        testTime1300,
			want:       true,
		},
		{
			name:       "dtstart and duration, range starts after effective end",
			properties: []string{testTodoStartProp, testTodoDurationProp},
			start:      testTime1201,
			end:        testTime1300,
			want:       false,
		},
		{
			name:       "dtstart and due, range ends at dtstart",
			properties: []string{testTodoStartProp, testTodoDue1200Prop},
			start:      testTime0900,
			end:        testTime1000,
			want:       false,
		},
		{
			name:       "dtstart and due, range covers dtstart",
			properties: []string{testTodoStartProp, testTodoDue1200Prop},
			start:      testTime0900,
			end:        testTime1001,
			want:       true,
		},
		{
			name:       "dtstart and due, range starts at due",
			properties: []string{testTodoStartProp, testTodoDue1200Prop},
			start:      testTime1200,
			end:        testTime1300,
			want:       false,
		},
		{
			name:       "dtstart and due, range before dtstart",
			properties: []string{testTodoStartProp, testTodoDue1200Prop},
			start:      testTime0900,
			end:        testTime0930,
			want:       false,
		},
		{
			name:       "dtstart only, range starts at dtstart",
			properties: []string{testTodoStartProp},
			start:      testTime1000,
			end:        testTime1100,
			want:       true,
		},
		{
			name:       "dtstart only, range ends at dtstart",
			properties: []string{testTodoStartProp},
			start:      testTime0900,
			end:        testTime1000,
			want:       false,
		},
		{
			name:       "due only, range ends at due",
			properties: []string{testTodoDue1000Prop},
			start:      testTime0900,
			end:        testTime1000,
			want:       true,
		},
		{
			name:       "due only, range starts at due",
			properties: []string{testTodoDue1000Prop},
			start:      testTime1000,
			end:        testTime1100,
			want:       false,
		},
		{
			name:       "due only, range before due",
			properties: []string{testTodoDue1000Prop},
			start:      testTime0900,
			end:        testTime0930,
			want:       false,
		},
		{
			name:       "due only, open range starts before due",
			properties: []string{testTodoDue1000Prop},
			start:      testTime0900,
			want:       true,
		},
		{
			name:       "due only, open range starts at due",
			properties: []string{testTodoDue1000Prop},
			start:      testTime1000,
			want:       false,
		},
		{
			name:       "completed and created, range before created",
			properties: []string{testTodoCompletedProp, testTodoCreatedProp},
			start:      testTime0800,
			end:        testTime0900,
			want:       false,
		},
		{
			name:       "completed and created, range covers created",
			properties: []string{testTodoCompletedProp, testTodoCreatedProp},
			start:      testTime0900,
			end:        testTime1001,
			want:       true,
		},
		{
			name:       "completed and created, range covers completed",
			properties: []string{testTodoCompletedProp, testTodoCreatedProp},
			start:      testTime1200,
			end:        testTime1300,
			want:       true,
		},
		{
			name:       "completed and created, range after completed",
			properties: []string{testTodoCompletedProp, testTodoCreatedProp},
			start:      testTime1201,
			end:        testTime1300,
			want:       false,
		},
		{
			name:       "completed only, range ends at completed",
			properties: []string{testTodoCompletedProp},
			start:      testTime0900,
			end:        testTime1200,
			want:       true,
		},
		{
			name:       "completed only, range after completed",
			properties: []string{testTodoCompletedProp},
			start:      testTime1201,
			end:        testTime1300,
			want:       false,
		},
		// Cases from Radicale's time-range tests.
		{
			name: "created and completed reversed, range ends at earlier date",
			properties: []string{
				testTodoCreated1200Prop,
				testTodoCompleted1000Prop,
			},
			start: testTime0900,
			end:   testTime1000,
			want:  true,
		},
		{
			name: "created and completed reversed, range starts at later date",
			properties: []string{
				testTodoCreated1200Prop,
				testTodoCompleted1000Prop,
			},
			start: testTime1200,
			end:   testTime1300,
			want:  true,
		},
		{
			name: "created and completed reversed, range after both dates",
			properties: []string{
				testTodoCreated1200Prop,
				testTodoCompleted1000Prop,
			},
			start: testTime1201,
			end:   testTime1300,
			want:  false,
		},
		{
			name: "dtstart and due take precedence over created and completed",
			properties: []string{
				testTodoStartProp,
				testTodoDue1200Prop,
				"CREATED:20060104T090000Z",
				"COMPLETED:20060104T093000Z",
			},
			start: testTime1001,
			end:   testTime1100,
			want:  true,
		},
		{
			name: "created and completed do not match when dtstart and due are present",
			properties: []string{
				testTodoStartProp,
				testTodoDue1200Prop,
				"CREATED:20060104T090000Z",
				"COMPLETED:20060104T093000Z",
			},
			start: testTime0900,
			end:   testTime0930,
			want:  false,
		},
		{
			name:       "created only, range ends after created",
			properties: []string{testTodoCreatedProp},
			start:      testTime0900,
			end:        testTime1001,
			want:       true,
		},
		{
			name:       "created only, range ends at created",
			properties: []string{testTodoCreatedProp},
			start:      testTime0900,
			end:        testTime1000,
			want:       false,
		},
		{
			name:       "no time properties always matches",
			properties: []string{"SUMMARY:Task"},
			start:      testTime0900,
			end:        testTime1000,
			want:       true,
		},
		{
			name:       "created only, open range always matches",
			properties: []string{testTodoCreatedProp},
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cal := mustParseICS(t, todoCalendar(tt.properties...))
			query := timeRangeQuery(t, tt.start, tt.end)
			query.Comps[0].Name = testTodoComponent

			match, err := MatchCalendar(query, cal)
			require.NoError(t, err)
			assert.Equal(t, tt.want, match)
		})
	}
}

func TestMatchCalendarVTodoRecurrence(t *testing.T) {
	t.Parallel()

	// Cases from Apple CalDAVTester and Radicale time-range tests.
	tests := []struct {
		name       string
		properties []string
		start      string
		end        string
		want       bool
	}{
		{
			name: "due recurrence instance in range",
			properties: []string{
				"DUE:20060102T160000Z",
				"RRULE:FREQ=DAILY;COUNT=5",
			},
			start: "20060105T000000Z",
			end:   "20060106T000000Z",
			want:  true,
		},
		{
			name: "due recurrence exhausted",
			properties: []string{
				"DUE:20060102T160000Z",
				"RRULE:FREQ=DAILY;COUNT=5",
			},
			start: "20060107T000000Z",
			end:   "20060108T000000Z",
			want:  false,
		},
		{
			name: "dtstart and duration recurrence instance in range",
			properties: []string{
				"DTSTART:20060101T100000Z",
				testTodoDurationProp,
				"RRULE:FREQ=DAILY;COUNT=3",
			},
			start: "20060102T110000Z",
			end:   "20060102T120000Z",
			want:  true,
		},
		{
			name: "dtstart and duration recurrence exhausted",
			properties: []string{
				"DTSTART:20060101T100000Z",
				testTodoDurationProp,
				"RRULE:FREQ=DAILY;COUNT=3",
			},
			start: "20060104T110000Z",
			end:   "20060104T120000Z",
			want:  false,
		},
		{
			name: "dtstart and due monthly recurrence instance in range",
			properties: []string{
				testTodoStartProp,
				testTodoDue1200Prop,
				"RRULE:FREQ=MONTHLY",
			},
			start: "20060204T110000Z",
			end:   "20060204T120000Z",
			want:  true,
		},
		{
			name: "dtstart and due monthly recurrence instance outside range",
			properties: []string{
				testTodoStartProp,
				testTodoDue1200Prop,
				"RRULE:FREQ=MONTHLY",
			},
			start: "20060208T110000Z",
			end:   "20060208T120000Z",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cal := mustParseICS(t, todoCalendar(tt.properties...))
			query := timeRangeQuery(t, tt.start, tt.end)
			query.Comps[0].Name = testTodoComponent

			match, err := MatchCalendar(query, cal)
			require.NoError(t, err)
			assert.Equal(t, tt.want, match)
		})
	}
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
