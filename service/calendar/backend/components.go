package backend

import (
	"slices"

	ics "github.com/arran4/golang-ical"

	"github.com/teapotovh/teapot/lib/webdav/caldav"
)

func componentType(component ics.Component) string {
	switch component.(type) {
	case *ics.VEvent:
		return string(ics.ComponentVEvent)
	case *ics.VTodo:
		return string(ics.ComponentVTodo)
	case *ics.VJournal:
		return string(ics.ComponentVJournal)
	case *ics.VBusy:
		return string(ics.ComponentVFreeBusy)
	default:
		return ""
	}
}

func checkSupportedComponents(set []string, calendar *ics.Calendar) error {
	if len(set) == 0 {
		return nil
	}

	for _, component := range calendar.Components {
		kind := componentType(component)
		if kind == "" {
			continue
		}

		if !slices.Contains(set, kind) {
			return caldav.NewPreconditionError(caldav.PreconditionSupportedCalendarComponent)
		}
	}

	return nil
}
