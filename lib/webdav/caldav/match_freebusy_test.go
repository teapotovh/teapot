package caldav

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testFreebusyComponent = "VFREEBUSY"

// freebusyPeriodsOnly has the FREEBUSY properties from the RFC 4791 fixture
// without DTSTART and DTEND.
const freebusyPeriodsOnly = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Example Corp.//CalDAV Client//EN
BEGIN:VFREEBUSY
UID:periods@example.com
DTSTAMP:20050530T123421Z
FREEBUSY:20060102T100000Z/20060102T120000Z
FREEBUSY;FBTYPE=BUSY-TENTATIVE:20060102T100000Z/20060102T120000Z
FREEBUSY:20060103T100000Z/PT2H
FREEBUSY;FBTYPE=BUSY-UNAVAILABLE:20060105T100000Z/20060105T120000Z
END:VFREEBUSY
END:VCALENDAR`

// freebusyEmpty has neither DTSTART, DTEND nor FREEBUSY properties.
const freebusyEmpty = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Example Corp.//CalDAV Client//EN
BEGIN:VFREEBUSY
UID:empty@example.com
DTSTAMP:20050530T123421Z
END:VFREEBUSY
END:VCALENDAR`

func freebusyQuery(t *testing.T, start, end string) CompFilter {
	t.Helper()

	return CompFilter{
		Name: testCalendarComponent,
		Comps: []CompFilter{{
			Name:  testFreebusyComponent,
			Start: optionalICSTime(t, start),
			End:   optionalICSTime(t, end),
		}},
	}
}

func TestMatchCalendarFreebusyTimeRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		data  string
		start string
		end   string
		want  bool
	}{
		{
			name:  "range within dtstart and dtend",
			data:  rfc4791Freebusy,
			start: "20060102T000000Z",
			end:   "20060103T000000Z",
			want:  true,
		},
		{
			name:  "range end equals dtstart",
			data:  rfc4791Freebusy,
			start: "20051231T000000Z",
			end:   "20060101T000000Z",
			want:  false,
		},
		{
			name:  "range start equals dtend",
			data:  rfc4791Freebusy,
			start: "20060108T000000Z",
			end:   "20060109T000000Z",
			want:  true,
		},
		{
			name:  "range starts after dtend",
			data:  rfc4791Freebusy,
			start: "20060109T000000Z",
			end:   "20060110T000000Z",
			want:  false,
		},
		{
			name:  "dtstart and dtend ignore freebusy periods",
			data:  rfc4791Freebusy,
			start: "20050531T230000Z",
			end:   "20050601T010000Z",
			want:  false,
		},
		{
			name:  "period overlaps range",
			data:  freebusyPeriodsOnly,
			start: "20060102T110000Z",
			end:   "20060102T130000Z",
			want:  true,
		},
		{
			name:  "range end equals period start",
			data:  freebusyPeriodsOnly,
			start: "20060102T080000Z",
			end:   "20060102T100000Z",
			want:  false,
		},
		{
			name:  "range start equals period end",
			data:  freebusyPeriodsOnly,
			start: "20060102T120000Z",
			end:   "20060102T140000Z",
			want:  false,
		},
		{
			name:  "period with duration overlaps range",
			data:  freebusyPeriodsOnly,
			start: "20060103T110000Z",
			end:   "20060103T130000Z",
			want:  true,
		},
		{
			name:  "busy unavailable period matches",
			data:  freebusyPeriodsOnly,
			start: "20060105T110000Z",
			end:   "20060105T130000Z",
			want:  true,
		},
		{
			name:  "no period overlaps range",
			data:  freebusyPeriodsOnly,
			start: "20060104T000000Z",
			end:   "20060105T000000Z",
			want:  false,
		},
		{
			name:  "no dtstart dtend or freebusy",
			data:  freebusyEmpty,
			start: "20060101T000000Z",
			end:   "20060102T000000Z",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cal := mustParseICS(t, tt.data)
			match, err := MatchCalendar(freebusyQuery(t, tt.start, tt.end), cal, time.UTC)
			require.NoError(t, err)
			assert.Equal(t, tt.want, match)
		})
	}
}
