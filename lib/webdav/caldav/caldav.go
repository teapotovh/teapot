// Package caldav provides a client and server CalDAV implementation.
//
// CalDAV is defined in RFC 4791.
package caldav

import (
	"errors"
	"fmt"
	"time"

	"github.com/emersion/go-ical"

	"github.com/teapotovh/teapot/lib/webdav"
	"github.com/teapotovh/teapot/lib/webdav/internal"
)

var (
	ErrUnexpectedMethod      = errors.New("calendar resource must not specify METHOD property")
	ErrConflictingEventTypes = errors.New("conflicting event types in calendar")
	ErrConflictingUIDs       = errors.New("conflicting UID values in calendar")
)

var CapabilityCalendar = webdav.Capability("calendar-access")

func NewCalendarHomeSet(path string) webdav.BackendSuppliedHomeSet {
	return &calendarHomeSet{Href: internal.Href{Path: path}}
}

// ValidateCalendarObject checks the validity of a calendar object according to
// the contraints laid out in RFC 4791 section 4.1 and returns the only event
// type and UID occurring in this calendar, or an error if the calendar could
// not be validated.
func ValidateCalendarObject(cal *ical.Calendar) (eventType string, uid string, err error) {
	// Calendar object resources contained in calendar collections
	// MUST NOT specify the iCalendar METHOD property.
	if prop := cal.Props.Get(ical.PropMethod); prop != nil {
		return "", "", ErrUnexpectedMethod
	}

	for _, comp := range cal.Children {
		// Calendar object resources contained in calendar collections
		// MUST NOT contain more than one type of calendar component
		// (e.g., VEVENT, VTODO, VJOURNAL, VFREEBUSY, etc.) with the
		// exception of VTIMEZONE components, which MUST be specified
		// for each unique TZID parameter value specified in the
		// iCalendar object.
		if comp.Name != ical.CompTimezone {
			if eventType == "" {
				eventType = comp.Name
			}

			if eventType != comp.Name {
				return "", "", fmt.Errorf("%w: %s, %s", ErrConflictingEventTypes, eventType, comp.Name)
			}
			// TODO check VTIMEZONE for each TZID?
		}

		// Calendar components in a calendar collection that have
		// different UID property values MUST be stored in separate
		// calendar object resources.
		compUID, err := comp.Props.Text(ical.PropUID)
		if err != nil {
			return "", "", fmt.Errorf("error checking component UID: %w", err)
		}

		if uid == "" {
			uid = compUID
		}

		if compUID != "" && uid != compUID {
			return "", "", fmt.Errorf("%w: %s, %s", ErrConflictingUIDs, uid, compUID)
		}
	}

	return eventType, uid, nil
}

type Calendar struct {
	Path                  string
	Name                  string
	Description           string
	MaxResourceSize       int64
	SupportedComponentSet []string
}

type CalendarCompRequest struct {
	Name string

	AllProps bool
	Props    []string

	AllComps bool
	Comps    []CalendarCompRequest

	Expand *CalendarExpandRequest
}

type CalendarExpandRequest struct {
	Start, End time.Time
}

type CompFilter struct {
	Name         string
	IsNotDefined bool
	Start, End   time.Time
	Props        []PropFilter
	Comps        []CompFilter
}

type ParamFilter struct {
	Name         string
	IsNotDefined bool
	TextMatch    *TextMatch
}

type PropFilter struct {
	Name         string
	IsNotDefined bool
	Start, End   time.Time
	TextMatch    *TextMatch
	ParamFilter  []ParamFilter
}

type TextMatch struct {
	Text            string
	NegateCondition bool
}

type CalendarQuery struct {
	CompRequest CalendarCompRequest
	CompFilter  CompFilter
}

type CalendarMultiGet struct {
	Paths       []string
	CompRequest CalendarCompRequest
}

type CalendarObject struct {
	Path          string
	ModTime       time.Time
	ContentLength int64
	ETag          string
	Data          *ical.Calendar
}
