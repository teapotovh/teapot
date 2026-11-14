package caldav

import (
	_ "embed"
)

//go:embed testdata/rfc4791-7.8.1-calendar-query.xml
var rfc4791CalendarQuery781 string

//go:embed testdata/rfc4791-7.8.2-calendar-query.xml
var rfc4791CalendarQuery782 string

//go:embed testdata/rfc4791-7.8.3-calendar-query.xml
var rfc4791CalendarQuery783 string

//go:embed testdata/rfc4791-7.8.6-calendar-query.xml
var rfc4791CalendarQuery786 string

//go:embed testdata/rfc4791-7.8.7-calendar-query.xml
var rfc4791CalendarQuery787 string

//go:embed testdata/rfc4791-7.8.9-calendar-query.xml
var rfc4791CalendarQuery789 string

//go:embed testdata/rfc4791-7.9.1-calendar-multiget.xml
var rfc4791CalendarMultiget791 string

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
