package caldav

import (
	"errors"
	"fmt"
	"strings"
	"time"

	ics "github.com/arran4/golang-ical"
)

// ErrInvalidPeriod is returned for an invalid iCalendar period value.
var ErrInvalidPeriod = errors.New("invalid iCalendar period")

// freebusyMatcher matches a filter against a VFREEBUSY component.
type freebusyMatcher struct {
	comp *ics.VBusy
}

func newFreebusyMatcher(comp *ics.VBusy) componentMatcher {
	return freebusyMatcher{comp: comp}
}

func (m freebusyMatcher) match(filter CompFilter) (bool, error) {
	if !strings.EqualFold(string(ics.ComponentVFreeBusy), filter.Name) {
		return false, nil
	}

	if !filter.Start.IsZero() || !filter.End.IsZero() {
		match, err := matchFreebusyTimeRange(filter.Start, filter.End, m.comp)
		if err != nil {
			return false, fmt.Errorf("error while matching time: %w", err)
		}

		if !match {
			return false, nil
		}
	}

	return matchComponentFilters(
		filter,
		&m.comp.ComponentBase,
		time.Time{},
		time.Time{},
		time.UTC,
	)
}

func matchFreebusy(filter CompFilter, comp *ics.VBusy) (bool, error) {
	return newFreebusyMatcher(comp).match(filter)
}

// matchFreebusyTimeRange reports whether a VFREEBUSY overlaps the time range,
// as defined in RFC 4791, section 9.9.
func matchFreebusyTimeRange(start, end time.Time, comp *ics.VBusy) (bool, error) {
	base := &comp.ComponentBase

	if base.GetProperty(ics.ComponentPropertyDtStart) != nil &&
		base.GetProperty(ics.ComponentPropertyDtEnd) != nil {
		from, err := parseUTCTimeProperty(base, ics.ComponentPropertyDtStart)
		if err != nil {
			return false, fmt.Errorf("while parsing freebusy start time: %w", err)
		}

		to, err := parseUTCTimeProperty(base, ics.ComponentPropertyDtEnd)
		if err != nil {
			return false, fmt.Errorf("while parsing freebusy end time: %w", err)
		}

		// The VFREEBUSY DTEND is inclusive.
		return !to.Before(start) && (end.IsZero() || from.Before(end)), nil
	}

	for _, prop := range base.GetProperties(ics.ComponentPropertyFreebusy) {
		match, err := freebusyOverlap(prop.Value, start, end)
		if err != nil {
			return false, fmt.Errorf("while parsing freebusy period: %w", err)
		}

		if match {
			return true, nil
		}
	}

	return false, nil
}

// freebusyOverlap reports whether any period of a FREEBUSY value overlaps the
// time range.
func freebusyOverlap(value string, start, end time.Time) (bool, error) {
	for part := range strings.SplitSeq(value, ",") {
		from, to, err := parsePeriod(strings.TrimSpace(part))
		if err != nil {
			return false, err
		}

		if (start.IsZero() || start.Before(to)) && (end.IsZero() || end.After(from)) {
			return true, nil
		}
	}

	return false, nil
}

// parsePeriod parses an iCalendar period as defined in RFC 5545, section 3.3.9.
func parsePeriod(value string) (time.Time, time.Time, error) {
	startValue, endValue, ok := strings.Cut(value, "/")
	if !ok {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: %q", ErrInvalidPeriod, value)
	}

	from, err := time.Parse(dateWithUTCTimeLayout, startValue)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: %q", ErrInvalidPeriod, value)
	}

	if endValue != "" && (endValue[0] == 'P' || endValue[0] == '+' || endValue[0] == '-') {
		duration, err := parseICalDuration(endValue)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("%w: %q", ErrInvalidPeriod, value)
		}

		return from, duration.addTo(from), nil
	}

	to, err := time.Parse(dateWithUTCTimeLayout, endValue)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: %q", ErrInvalidPeriod, value)
	}

	return from, to, nil
}
