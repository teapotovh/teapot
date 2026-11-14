package calendar

import (
	_ "embed"
)

//go:embed testdata/calendar-query.xml
var calendarQueryXML string

//go:embed testdata/mkcalendar.xml
var mkcalendarXML string

//go:embed testdata/teapot-event.ics
var teapotEvent string
