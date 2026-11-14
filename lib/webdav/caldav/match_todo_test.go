package caldav

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
