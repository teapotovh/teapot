package caldav

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "embed"
)

//go:embed testdata/rfc4791-todo-alarm-7.8.5.ics
var rfc4791TodoAlarm string

//go:embed testdata/teapot-event-alarm.ics
var teapotEventAlarm string

func TestMatchCalendarValarmTimeRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		data   string
		parent string
		start  string
		end    string
		want   bool
	}{
		{
			name:   "RFC 4791 section 7.8.5 todo alarm in range",
			data:   rfc4791TodoAlarm,
			parent: testTodoComponent,
			start:  "20060106T100000Z",
			end:    "20060107T100000Z",
			want:   true,
		},
		{
			name:   "RFC 4791 section 7.8.5 todo alarm before range",
			data:   rfc4791TodoAlarm,
			parent: testTodoComponent,
			start:  "20060106T000000Z",
			end:    "20060106T100000Z",
			want:   false,
		},
		{
			name:   "event alarm relative to start in range",
			data:   teapotEventAlarm,
			parent: testEventComponent,
			start:  testTime0900,
			end:    testTime1000,
			want:   true,
		},
		{
			name:   "event alarm relative to start after range",
			data:   teapotEventAlarm,
			parent: testEventComponent,
			start:  testTime1000,
			end:    testTime1100,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			query := &CompFilter{
				Name: testCalendarComponent,
				Comps: []CompFilter{{
					Name: tt.parent,
					Comps: []CompFilter{{
						Name:  testAlarmComponent,
						Start: mustICSTime(t, tt.start),
						End:   mustICSTime(t, tt.end),
					}},
				}},
			}

			match, err := MatchCalendar(*query, mustParseICS(t, tt.data))
			require.NoError(t, err)
			assert.Equal(t, tt.want, match)
		})
	}
}
