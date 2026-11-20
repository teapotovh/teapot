package caldav

import (
	"encoding/xml"
	"strings"
	"testing"
	"time"

	ics "github.com/arran4/golang-ical"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testTimeZoneCalendar = "BEGIN:VCALENDAR\r\n" +
	"VERSION:2.0\r\n" +
	"PRODID:-//Test//Test//EN\r\n" +
	"BEGIN:VTIMEZONE\r\n" +
	"TZID:US/Eastern\r\n" +
	"BEGIN:STANDARD\r\n" +
	"DTSTART:19671029T020000\r\n" +
	"RRULE:FREQ=YEARLY;BYDAY=-1SU;BYMONTH=10\r\n" +
	"TZOFFSETFROM:-0400\r\n" +
	"TZOFFSETTO:-0500\r\n" +
	"TZNAME:EST\r\n" +
	"END:STANDARD\r\n" +
	"BEGIN:DAYLIGHT\r\n" +
	"DTSTART:19870405T020000\r\n" +
	"RRULE:FREQ=YEARLY;BYDAY=1SU;BYMONTH=4\r\n" +
	"TZOFFSETFROM:-0500\r\n" +
	"TZOFFSETTO:-0400\r\n" +
	"TZNAME:EDT\r\n" +
	"END:DAYLIGHT\r\n" +
	"END:VTIMEZONE\r\n" +
	"END:VCALENDAR\r\n"

func freeBusyObject(t *testing.T, event string) CalendarObject {
	t.Helper()

	body := "BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"PRODID:-//Test//Test//EN\r\n" +
		event +
		"END:VCALENDAR\r\n"

	cal, err := ics.ParseCalendar(strings.NewReader(body))
	require.NoError(t, err)

	return CalendarObject{Path: "event.ics", Data: cal}
}

func freeBusyRange() (time.Time, time.Time) {
	return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
}

func utcTime(t *testing.T, hour int) time.Time {
	t.Helper()

	return time.Date(2026, 1, 1, hour, 0, 0, 0, time.UTC)
}

func TestComputeFreeBusyBusyTypes(t *testing.T) {
	t.Parallel()

	start, end := freeBusyRange()

	cos := []CalendarObject{
		freeBusyObject(t, "BEGIN:VEVENT\r\n"+
			"UID:busy@example.com\r\n"+
			"DTSTAMP:20260101T000000Z\r\n"+
			"DTSTART:20260101T100000Z\r\n"+
			"DTEND:20260101T110000Z\r\n"+
			"END:VEVENT\r\n"),
		freeBusyObject(t, "BEGIN:VEVENT\r\n"+
			"UID:tentative@example.com\r\n"+
			"DTSTAMP:20260101T000000Z\r\n"+
			"DTSTART:20260101T120000Z\r\n"+
			"DTEND:20260101T130000Z\r\n"+
			"STATUS:TENTATIVE\r\n"+
			"END:VEVENT\r\n"),
	}

	got, err := computeFreeBusy(cos, start, end, time.UTC)
	require.NoError(t, err)

	assert.Equal(t, []freeBusyPeriod{
		{Start: utcTime(t, 10), End: utcTime(t, 11)},
	}, got[freeBusyTypeBusy])
	assert.Equal(t, []freeBusyPeriod{
		{Start: utcTime(t, 12), End: utcTime(t, 13)},
	}, got[freeBusyTypeTentative])
}

func TestComputeFreeBusySkipsCancelledAndTransparent(t *testing.T) {
	t.Parallel()

	start, end := freeBusyRange()

	cos := []CalendarObject{
		freeBusyObject(t, "BEGIN:VEVENT\r\n"+
			"UID:cancelled@example.com\r\n"+
			"DTSTAMP:20260101T000000Z\r\n"+
			"DTSTART:20260101T100000Z\r\n"+
			"DTEND:20260101T110000Z\r\n"+
			"STATUS:CANCELLED\r\n"+
			"END:VEVENT\r\n"),
		freeBusyObject(t, "BEGIN:VEVENT\r\n"+
			"UID:transparent@example.com\r\n"+
			"DTSTAMP:20260101T000000Z\r\n"+
			"DTSTART:20260101T120000Z\r\n"+
			"DTEND:20260101T130000Z\r\n"+
			"TRANSP:TRANSPARENT\r\n"+
			"END:VEVENT\r\n"),
		freeBusyObject(t, "BEGIN:VEVENT\r\n"+
			"UID:tentative-transparent@example.com\r\n"+
			"DTSTAMP:20260101T000000Z\r\n"+
			"DTSTART:20260101T140000Z\r\n"+
			"DTEND:20260101T150000Z\r\n"+
			"STATUS:TENTATIVE\r\n"+
			"TRANSP:TRANSPARENT\r\n"+
			"END:VEVENT\r\n"),
	}

	got, err := computeFreeBusy(cos, start, end, time.UTC)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestComputeFreeBusyAllDay(t *testing.T) {
	t.Parallel()

	start, end := freeBusyRange()

	cos := []CalendarObject{
		freeBusyObject(t, "BEGIN:VEVENT\r\n"+
			"UID:allday@example.com\r\n"+
			"DTSTAMP:20260101T000000Z\r\n"+
			"DTSTART;VALUE=DATE:20260101\r\n"+
			"END:VEVENT\r\n"),
	}

	got, err := computeFreeBusy(cos, start, end, time.UTC)
	require.NoError(t, err)
	assert.Equal(t, []freeBusyPeriod{
		{Start: start, End: end},
	}, got[freeBusyTypeBusy])
}

func TestComputeFreeBusyClipsPeriods(t *testing.T) {
	t.Parallel()

	start, end := freeBusyRange()

	cos := []CalendarObject{
		freeBusyObject(t, "BEGIN:VEVENT\r\n"+
			"UID:before@example.com\r\n"+
			"DTSTAMP:20251231T000000Z\r\n"+
			"DTSTART:20251231T230000Z\r\n"+
			"DTEND:20260101T010000Z\r\n"+
			"END:VEVENT\r\n"),
		freeBusyObject(t, "BEGIN:VEVENT\r\n"+
			"UID:after@example.com\r\n"+
			"DTSTAMP:20260101T000000Z\r\n"+
			"DTSTART:20260101T230000Z\r\n"+
			"DTEND:20260102T010000Z\r\n"+
			"END:VEVENT\r\n"),
	}

	got, err := computeFreeBusy(cos, start, end, time.UTC)
	require.NoError(t, err)
	assert.Equal(t, []freeBusyPeriod{
		{Start: start, End: utcTime(t, 1)},
		{Start: utcTime(t, 23), End: end},
	}, got[freeBusyTypeBusy])
}

func TestComputeFreeBusyMergesPeriods(t *testing.T) {
	t.Parallel()

	start, end := freeBusyRange()

	cos := []CalendarObject{
		freeBusyObject(t, "BEGIN:VEVENT\r\n"+
			"UID:first@example.com\r\n"+
			"DTSTAMP:20260101T000000Z\r\n"+
			"DTSTART:20260101T100000Z\r\n"+
			"DTEND:20260101T110000Z\r\n"+
			"END:VEVENT\r\n"),
		freeBusyObject(t, "BEGIN:VEVENT\r\n"+
			"UID:second@example.com\r\n"+
			"DTSTAMP:20260101T000000Z\r\n"+
			"DTSTART:20260101T110000Z\r\n"+
			"DTEND:20260101T120000Z\r\n"+
			"END:VEVENT\r\n"),
		freeBusyObject(t, "BEGIN:VEVENT\r\n"+
			"UID:third@example.com\r\n"+
			"DTSTAMP:20260101T000000Z\r\n"+
			"DTSTART:20260101T114500Z\r\n"+
			"DTEND:20260101T130000Z\r\n"+
			"END:VEVENT\r\n"),
	}

	got, err := computeFreeBusy(cos, start, end, time.UTC)
	require.NoError(t, err)
	assert.Equal(t, []freeBusyPeriod{
		{Start: utcTime(t, 10), End: utcTime(t, 13)},
	}, got[freeBusyTypeBusy])
}

func TestComputeFreeBusyCalendarTimeZone(t *testing.T) {
	t.Parallel()

	location, err := time.LoadLocation("US/Eastern")
	require.NoError(t, err)

	start, end := freeBusyRange()

	cos := []CalendarObject{
		freeBusyObject(t, "BEGIN:VEVENT\r\n"+
			"UID:floating@example.com\r\n"+
			"DTSTAMP:20260101T000000Z\r\n"+
			"DTSTART:20260101T010000\r\n"+
			"DTEND:20260101T020000\r\n"+
			"END:VEVENT\r\n"),
		freeBusyObject(t, "BEGIN:VEVENT\r\n"+
			"UID:allday@example.com\r\n"+
			"DTSTAMP:20260101T000000Z\r\n"+
			"DTSTART;VALUE=DATE:20260101\r\n"+
			"STATUS:TENTATIVE\r\n"+
			"END:VEVENT\r\n"),
	}

	got, err := computeFreeBusy(cos, start, end, location)
	require.NoError(t, err)

	assert.Equal(t, []freeBusyPeriod{
		{
			Start: time.Date(2026, 1, 1, 1, 0, 0, 0, location),
			End:   time.Date(2026, 1, 1, 2, 0, 0, 0, location),
		},
	}, got[freeBusyTypeBusy])
	assert.Equal(t, []freeBusyPeriod{
		{
			Start: time.Date(2026, 1, 1, 0, 0, 0, 0, location),
			End:   end,
		},
	}, got[freeBusyTypeTentative])
}

func TestCalendarTimeZone(t *testing.T) {
	t.Parallel()

	cal := &Calendar{
		DeadProperties: map[xml.Name]string{
			calendarTimeZoneName: `<calendar-timezone xmlns="urn:ietf:params:xml:ns:caldav">` +
				testTimeZoneCalendar +
				`</calendar-timezone>`,
		},
	}

	location, err := calendarLocation(cal)
	require.NoError(t, err)

	_, offset := time.Date(2026, 1, 1, 0, 0, 0, 0, location).Zone()
	assert.Equal(t, -5*60*60, offset)

	location, err = calendarLocation(&Calendar{})
	require.NoError(t, err)
	assert.Equal(t, time.UTC, location)
}

func TestWriteFreeBusyFoldsLines(t *testing.T) {
	t.Parallel()

	start, end := freeBusyRange()

	var periods []freeBusyPeriod

	for i := range 10 {
		periodStart := start.Add(time.Duration(i) * time.Hour)
		periods = append(periods, freeBusyPeriod{
			Start: periodStart,
			End:   periodStart.Add(30 * time.Minute),
		})
	}

	var b strings.Builder
	require.NoError(t, writeFreeBusy(&b, start, end, map[string][]freeBusyPeriod{
		freeBusyTypeBusy: periods,
	}))

	body := b.String()
	require.Contains(t, body, "\r\n ")

	for line := range strings.SplitSeq(body, "\r\n") {
		assert.LessOrEqual(t, len(line), 75, "line longer than 75 octets: %q", line)
	}
}
