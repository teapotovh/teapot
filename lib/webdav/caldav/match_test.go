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

const (
	testCalendarComponent = "VCALENDAR"
	testEventComponent    = "VEVENT"
	testTodoComponent     = "VTODO"
	testAlarmComponent    = "VALARM"

	abcd3Path = "/bernard/work/abcd3.ics"
)

type appendixBFixture struct {
	name string
	uid  string
}

var appendixBFixtures = []appendixBFixture{
	{name: "abcd1.ics", uid: "74855313FA803DA593CD579A@example.com"},
	{name: "abcd2.ics", uid: "00959BC664CA650E933C892C@example.com"},
	{name: "abcd3.ics", uid: "DC6C50A017428C5216A2F1CD@example.com"},
	{name: "abcd4.ics", uid: "DDDEEB7915FA61233B861457@example.com"},
	{name: "abcd5.ics", uid: "E10BA47467C5C69BB74E8720@example.com"},
	{name: "abcd6.ics", uid: "E10BA47467C5C69BB74E8722@example.com"},
	{name: "abcd7.ics", uid: "E10BA47467C5C69BB74E8725@example.com"},
	{name: "abcd8.ics", uid: "76ef34-54a3d2@example.com"},
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
		cal := mustParseICS(t, testdata(t, fixture.name))
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
			want: []string{"/bernard/work/abcd2.ics", abcd3Path},
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
			want: []string{abcd3Path},
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
			want: []string{abcd3Path},
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
				abcd3Path,
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
				abcd3Path,
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
				abcd3Path,
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
			want: []string{abcd3Path},
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
		name    string
		fixture string
		start   string
		end     string
		want    bool
	}{
		{
			name:    "range covers event",
			fixture: "teapot-overlapping.ics",
			start:   "20060104T090000Z",
			end:     "20060104T120000Z",
			want:    true,
		},
		{
			name:    "range start equals event start",
			fixture: "teapot-overlapping.ics",
			start:   "20060104T100000Z",
			end:     "20060104T120000Z",
			want:    true,
		},
		{
			name:    "range start equals event end",
			fixture: "teapot-overlapping.ics",
			start:   "20060104T110000Z",
			end:     "20060104T120000Z",
			want:    false,
		},
		{
			name:    "range end equals event start",
			fixture: "teapot-overlapping.ics",
			start:   "20060104T090000Z",
			end:     "20060104T100000Z",
			want:    false,
		},
		{
			name:    "range end equals event end",
			fixture: "teapot-overlapping.ics",
			start:   "20060104T103000Z",
			end:     "20060104T110000Z",
			want:    true,
		},
		{
			name:    "open-ended range overlaps event",
			fixture: "teapot-overlapping.ics",
			start:   "20060104T103000Z",
			end:     "",
			want:    true,
		},
		{
			name:    "event before open-ended range",
			fixture: "teapot-overlapping.ics",
			start:   "20060104T120000Z",
			end:     "",
			want:    false,
		},
		{
			name:    "zero-duration event at range start",
			fixture: "teapot-zero-duration.ics",
			start:   "20060104T100000Z",
			end:     "20060104T110000Z",
			want:    true,
		},
		{
			name:    "zero-duration event inside range",
			fixture: "teapot-zero-duration.ics",
			start:   "20060104T090000Z",
			end:     "20060104T110000Z",
			want:    true,
		},
		{
			name:    "zero-duration event at range end",
			fixture: "teapot-zero-duration.ics",
			start:   "20060104T090000Z",
			end:     "20060104T100000Z",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cal := mustParseICS(t, testdata(t, tt.fixture))
			match, err := MatchCalendar(timeRangeQuery(t, tt.start, tt.end), cal)
			require.NoError(t, err)
			assert.Equal(t, tt.want, match)
		})
	}
}

func TestMatchCalendarInvalidDuration(t *testing.T) {
	t.Parallel()

	cal := mustParseICS(t, testdata(t, "teapot-invalid-duration.ics"))

	_, err := MatchCalendar(timeRangeQuery(t, "20060104T000000Z", "20060105T000000Z"), cal)
	require.ErrorIs(t, err, ErrInvalidDuration)
}

func TestMatchCalendarRecurrence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		fixture string
		start   string
		end     string
		want    bool
	}{
		{
			name:    "recurring instance within range",
			fixture: "teapot-daily-rrule.ics",
			start:   "20060104T000000Z",
			end:     "20060105T000000Z",
			want:    true,
		},
		{
			name:    "recurrence count exhausted",
			fixture: "teapot-daily-rrule.ics",
			start:   "20060105T000000Z",
			end:     "20060106T000000Z",
			want:    false,
		},
		{
			name:    "excluded date does not match",
			fixture: "teapot-exdate.ics",
			start:   "20060103T090000Z",
			end:     "20060103T120000Z",
			want:    false,
		},
		{
			name:    "excluded date keeps other instances",
			fixture: "teapot-exdate.ics",
			start:   "20060104T090000Z",
			end:     "20060104T120000Z",
			want:    true,
		},
		{
			name:    "rdate instance within range",
			fixture: "teapot-rdate.ics",
			start:   "20060110T090000Z",
			end:     "20060110T120000Z",
			want:    true,
		},
		{
			name:    "rdate instance outside range",
			fixture: "teapot-rdate.ics",
			start:   "20060105T000000Z",
			end:     "20060106T000000Z",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cal := mustParseICS(t, testdata(t, tt.fixture))
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
