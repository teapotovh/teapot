package caldav

import (
	"strings"
	"testing"
	"time"

	ics "github.com/arran4/golang-ical"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "time/tzdata"
)

// Calendar objects from RFC 4791, Appendix B. Only abcd1.ics is modified:
// the "Description:" property is written as "DESCRIPTION:" so that the
// parser accepts it.
const abcd1 = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Example Corp.//CalDAV Client//EN
BEGIN:VTIMEZONE
LAST-MODIFIED:20040110T032845Z
TZID:US/Eastern
BEGIN:DAYLIGHT
DTSTART:20000404T020000
RRULE:FREQ=YEARLY;BYDAY=1SU;BYMONTH=4
TZNAME:EDT
TZOFFSETFROM:-0500
TZOFFSETTO:-0400
END:DAYLIGHT
BEGIN:STANDARD
DTSTART:20001026T020000
RRULE:FREQ=YEARLY;BYDAY=-1SU;BYMONTH=10
TZNAME:EST
TZOFFSETFROM:-0400
TZOFFSETTO:-0500
END:STANDARD
END:VTIMEZONE
BEGIN:VEVENT
DTSTAMP:20060206T001102Z
DTSTART;TZID=US/Eastern:20060102T100000
DURATION:PT1H
SUMMARY:Event #1
DESCRIPTION:Go Steelers!
UID:74855313FA803DA593CD579A@example.com
END:VEVENT
END:VCALENDAR
`

const abcd2 = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Example Corp.//CalDAV Client//EN
BEGIN:VTIMEZONE
LAST-MODIFIED:20040110T032845Z
TZID:US/Eastern
BEGIN:DAYLIGHT
DTSTART:20000404T020000
RRULE:FREQ=YEARLY;BYDAY=1SU;BYMONTH=4
TZNAME:EDT
TZOFFSETFROM:-0500
TZOFFSETTO:-0400
END:DAYLIGHT
BEGIN:STANDARD
DTSTART:20001026T020000
RRULE:FREQ=YEARLY;BYDAY=-1SU;BYMONTH=10
TZNAME:EST
TZOFFSETFROM:-0400
TZOFFSETTO:-0500
END:STANDARD
END:VTIMEZONE
BEGIN:VEVENT
DTSTAMP:20060206T001121Z
DTSTART;TZID=US/Eastern:20060102T120000
DURATION:PT1H
RRULE:FREQ=DAILY;COUNT=5
SUMMARY:Event #2
UID:00959BC664CA650E933C892C@example.com
END:VEVENT
BEGIN:VEVENT
DTSTAMP:20060206T001121Z
DTSTART;TZID=US/Eastern:20060104T140000
DURATION:PT1H
RECURRENCE-ID;TZID=US/Eastern:20060104T120000
SUMMARY:Event #2 bis
UID:00959BC664CA650E933C892C@example.com
END:VEVENT
END:VCALENDAR
`

const abcd3 = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Example Corp.//CalDAV Client//EN
BEGIN:VTIMEZONE
LAST-MODIFIED:20040110T032845Z
TZID:US/Eastern
BEGIN:DAYLIGHT
DTSTART:20000404T020000
RRULE:FREQ=YEARLY;BYDAY=1SU;BYMONTH=4
TZNAME:EDT
TZOFFSETFROM:-0500
TZOFFSETTO:-0400
END:DAYLIGHT
BEGIN:STANDARD
DTSTART:20001026T020000
RRULE:FREQ=YEARLY;BYDAY=-1SU;BYMONTH=10
TZNAME:EST
TZOFFSETFROM:-0400
TZOFFSETTO:-0500
END:STANDARD
END:VTIMEZONE
BEGIN:VEVENT
ATTENDEE;PARTSTAT=ACCEPTED;ROLE=CHAIR:mailto:cyrus@example.com
ATTENDEE;PARTSTAT=NEEDS-ACTION:mailto:lisa@example.com
DTSTAMP:20060206T001220Z
DTSTART;TZID=US/Eastern:20060104T100000
DURATION:PT1H
LAST-MODIFIED:20060206T001330Z
ORGANIZER:mailto:cyrus@example.com
SEQUENCE:1
STATUS:TENTATIVE
SUMMARY:Event #3
UID:DC6C50A017428C5216A2F1CD@example.com
END:VEVENT
END:VCALENDAR
`

const abcd4 = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Example Corp.//CalDAV Client//EN
BEGIN:VTODO
DTSTAMP:20060205T235335Z
DUE;VALUE=DATE:20060104
STATUS:NEEDS-ACTION
SUMMARY:Task #1
UID:DDDEEB7915FA61233B861457@example.com
BEGIN:VALARM
ACTION:AUDIO
TRIGGER;RELATED=START:-PT10M
END:VALARM
END:VTODO
END:VCALENDAR
`

const abcd5 = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Example Corp.//CalDAV Client//EN
BEGIN:VTODO
DTSTAMP:20060205T235300Z
DUE;VALUE=DATE:20060106
LAST-MODIFIED:20060205T235308Z
SEQUENCE:1
STATUS:NEEDS-ACTION
SUMMARY:Task #2
UID:E10BA47467C5C69BB74E8720@example.com
BEGIN:VALARM
ACTION:AUDIO
TRIGGER;RELATED=START:-PT10M
END:VALARM
END:VTODO
END:VCALENDAR
`

const abcd6 = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Example Corp.//CalDAV Client//EN
BEGIN:VTODO
COMPLETED:20051223T122322Z
DTSTAMP:20060205T235400Z
DUE;VALUE=DATE:20051225
LAST-MODIFIED:20060205T235308Z
SEQUENCE:1
STATUS:COMPLETED
SUMMARY:Task #3
UID:E10BA47467C5C69BB74E8722@example.com
END:VTODO
END:VCALENDAR
`

const abcd7 = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Example Corp.//CalDAV Client//EN
BEGIN:VTODO
DTSTAMP:20060205T235600Z
DUE;VALUE=DATE:20060101
LAST-MODIFIED:20060205T235308Z
SEQUENCE:1
STATUS:CANCELLED
SUMMARY:Task #4
UID:E10BA47467C5C69BB74E8725@example.com
END:VTODO
END:VCALENDAR
`

const abcd8 = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Example Corp.//CalDAV Client//EN
BEGIN:VFREEBUSY
ORGANIZER;CN="Bernard Desruisseaux":mailto:bernard@example.com
UID:76ef34-54a3d2@example.com
DTSTAMP:20050530T123421Z
DTSTART:20060101T000000Z
DTEND:20060108T000000Z
FREEBUSY:20050531T230000Z/20050601T010000Z
FREEBUSY;FBTYPE=BUSY-TENTATIVE:20060102T100000Z/20060102T120000Z
FREEBUSY:20060103T100000Z/20060103T120000Z
FREEBUSY:20060104T100000Z/20060104T120000Z
FREEBUSY;FBTYPE=BUSY-UNAVAILABLE:20060105T100000Z/20060105T120000Z
FREEBUSY:20060106T100000Z/20060106T120000Z
END:VFREEBUSY
END:VCALENDAR
`

const (
	testCalendarComponent = "VCALENDAR"
	testEventComponent    = "VEVENT"
	testTodoComponent     = "VTODO"
	testAlarmComponent    = "VALARM"
)

type appendixBFixture struct {
	name string
	uid  string
	data string
}

var appendixBFixtures = []appendixBFixture{
	{name: "abcd1.ics", uid: "74855313FA803DA593CD579A@example.com", data: abcd1},
	{name: "abcd2.ics", uid: "00959BC664CA650E933C892C@example.com", data: abcd2},
	{name: "abcd3.ics", uid: "DC6C50A017428C5216A2F1CD@example.com", data: abcd3},
	{name: "abcd4.ics", uid: "DDDEEB7915FA61233B861457@example.com", data: abcd4},
	{name: "abcd5.ics", uid: "E10BA47467C5C69BB74E8720@example.com", data: abcd5},
	{name: "abcd6.ics", uid: "E10BA47467C5C69BB74E8722@example.com", data: abcd6},
	{name: "abcd7.ics", uid: "E10BA47467C5C69BB74E8725@example.com", data: abcd7},
	{name: "abcd8.ics", uid: "76ef34-54a3d2@example.com", data: abcd8},
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
			Path: "/bernard/work/" + fixture.name,
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

func icsEvent(props ...string) string {
	return "BEGIN:VEVENT\n" + strings.Join(props, "\n") + "\nEND:VEVENT\n"
}

func eventCalendar(t *testing.T, event string) *ics.Calendar {
	t.Helper()

	return mustParseICS(t, "BEGIN:VCALENDAR\nVERSION:2.0\n"+event+"END:VCALENDAR\n")
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
			want: []string{"/bernard/work/abcd2.ics", "/bernard/work/abcd3.ics"},
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
			want: []string{"/bernard/work/abcd3.ics"},
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
			want: []string{"/bernard/work/abcd3.ics"},
		},
		{
			name: "7.8.8 events only",
			query: &CompFilter{
				Name:  testCalendarComponent,
				Comps: []CompFilter{{Name: testEventComponent}},
			},
			want: []string{
				"/bernard/work/abcd1.ics",
				"/bernard/work/abcd2.ics",
				"/bernard/work/abcd3.ics",
			},
		},
		{
			name: "7.8.10 unsupported property filter",
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
			want: []string{"/bernard/work/abcd1.ics", "/bernard/work/abcd2.ics"},
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
				"/bernard/work/abcd1.ics",
				"/bernard/work/abcd2.ics",
				"/bernard/work/abcd3.ics",
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
				"/bernard/work/abcd1.ics",
				"/bernard/work/abcd2.ics",
				"/bernard/work/abcd3.ics",
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
			want: []string{"/bernard/work/abcd3.ics"},
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

	overlapping := icsEvent(
		"UID:overlapping@example.com",
		"DTSTAMP:20060101T000000Z",
		"DTSTART:20060104T100000Z",
		"DTEND:20060104T110000Z",
	)
	zeroDuration := icsEvent(
		"UID:zero@example.com",
		"DTSTAMP:20060101T000000Z",
		"DTSTART:20060104T100000Z",
	)

	tests := []struct {
		name  string
		event string
		start string
		end   string
		want  bool
	}{
		{
			name:  "range covers event",
			event: overlapping,
			start: "20060104T090000Z",
			end:   "20060104T120000Z",
			want:  true,
		},
		{
			name:  "range start equals event start",
			event: overlapping,
			start: "20060104T100000Z",
			end:   "20060104T120000Z",
			want:  true,
		},
		{
			name:  "range start equals event end",
			event: overlapping,
			start: "20060104T110000Z",
			end:   "20060104T120000Z",
			want:  false,
		},
		{
			name:  "range end equals event start",
			event: overlapping,
			start: "20060104T090000Z",
			end:   "20060104T100000Z",
			want:  false,
		},
		{
			name:  "range end equals event end",
			event: overlapping,
			start: "20060104T103000Z",
			end:   "20060104T110000Z",
			want:  true,
		},
		{
			name:  "open-ended range overlaps event",
			event: overlapping,
			start: "20060104T103000Z",
			end:   "",
			want:  true,
		},
		{
			name:  "event before open-ended range",
			event: overlapping,
			start: "20060104T120000Z",
			end:   "",
			want:  false,
		},
		{
			name:  "zero-duration event at range start",
			event: zeroDuration,
			start: "20060104T100000Z",
			end:   "20060104T110000Z",
			want:  true,
		},
		{
			name:  "zero-duration event inside range",
			event: zeroDuration,
			start: "20060104T090000Z",
			end:   "20060104T110000Z",
			want:  true,
		},
		{
			name:  "zero-duration event at range end",
			event: zeroDuration,
			start: "20060104T090000Z",
			end:   "20060104T100000Z",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			match, err := MatchCalendar(timeRangeQuery(t, tt.start, tt.end), eventCalendar(t, tt.event))
			require.NoError(t, err)
			assert.Equal(t, tt.want, match)
		})
	}
}

func TestMatchCalendarRecurrence(t *testing.T) {
	t.Parallel()

	daily := icsEvent(
		"UID:daily@example.com",
		"DTSTAMP:20060101T000000Z",
		"DTSTART:20060102T100000Z",
		"DTEND:20060102T110000Z",
		"RRULE:FREQ=DAILY;COUNT=3",
	)
	excluded := icsEvent(
		"UID:excluded@example.com",
		"DTSTAMP:20060101T000000Z",
		"DTSTART:20060102T100000Z",
		"DTEND:20060102T110000Z",
		"RRULE:FREQ=DAILY;COUNT=3",
		"EXDATE:20060103T100000Z",
	)
	additional := icsEvent(
		"UID:additional@example.com",
		"DTSTAMP:20060101T000000Z",
		"DTSTART:20060102T100000Z",
		"DTEND:20060102T110000Z",
		"RDATE:20060110T100000Z",
	)

	tests := []struct {
		name  string
		event string
		start string
		end   string
		want  bool
	}{
		{
			name:  "recurring instance within range",
			event: daily,
			start: "20060104T000000Z",
			end:   "20060105T000000Z",
			want:  true,
		},
		{
			name:  "recurrence count exhausted",
			event: daily,
			start: "20060105T000000Z",
			end:   "20060106T000000Z",
			want:  false,
		},
		{
			name:  "excluded date does not match",
			event: excluded,
			start: "20060103T090000Z",
			end:   "20060103T120000Z",
			want:  false,
		},
		{
			name:  "excluded date keeps other instances",
			event: excluded,
			start: "20060104T090000Z",
			end:   "20060104T120000Z",
			want:  true,
		},
		{
			name:  "rdate instance within range",
			event: additional,
			start: "20060110T090000Z",
			end:   "20060110T120000Z",
			want:  true,
		},
		{
			name:  "rdate instance outside range",
			event: additional,
			start: "20060105T000000Z",
			end:   "20060106T000000Z",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			match, err := MatchCalendar(timeRangeQuery(t, tt.start, tt.end), eventCalendar(t, tt.event))
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
	assert.Equal(t, []string{"/bernard/work/abcd4.ics"}, objectPaths(got))
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
	assert.Equal(t, []string{"/bernard/work/abcd4.ics", "/bernard/work/abcd5.ics"}, objectPaths(got))
}
