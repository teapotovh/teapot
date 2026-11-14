package caldav

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	ics "github.com/arran4/golang-ical"
)

var ErrInvalidDuration = errors.New("invalid iCalendar duration")

// eventEndFromDuration computes the effective end of an event that does not
// define DTEND, using its DURATION property as described in RFC 4791,
// section 9.9. Events without DURATION have zero duration.
func eventEndFromDuration(comp *ics.VEvent, eventStart time.Time) (time.Time, error) {
	prop := comp.GetProperty(ics.ComponentPropertyDuration)
	if prop == nil {
		return eventStart, nil
	}

	duration, err := parseICalDuration(prop.Value)
	if err != nil {
		return time.Time{}, fmt.Errorf("while parsing event duration: %w", err)
	}

	if duration.days < 0 || duration.exact < 0 {
		return time.Time{}, fmt.Errorf("%w: %q", ErrInvalidDuration, prop.Value)
	}

	return duration.addTo(eventStart), nil
}

// icalDuration is a duration as defined in RFC 5545, section 3.3.6. Days are
// nominal and keep the wall clock time across DST transitions. Hours, minutes
// and seconds are exact. Weeks are converted to days.
type icalDuration struct {
	days  int
	exact time.Duration
}

func (d icalDuration) addTo(t time.Time) time.Time {
	return t.AddDate(0, 0, d.days).Add(d.exact)
}

// Date units are nominal, time units are exact.
var (
	icalDurationDateUnits = map[byte]int{'W': 7, 'D': 1}
	icalDurationTimeUnits = map[byte]time.Duration{'H': time.Hour, 'M': time.Minute, 'S': time.Second}
)

// parseICalDuration parses a duration value as defined in RFC 5545, section
// 3.3.6. The input is case-insensitive. A bare "P" is accepted for parity with
// the previous parser.
func parseICalDuration(value string) (icalDuration, error) {
	rest := strings.ToUpper(value)

	rest, negative := strings.CutPrefix(rest, "-")
	if !negative {
		rest, _ = strings.CutPrefix(rest, "+")
	}

	rest, ok := strings.CutPrefix(rest, "P")
	if !ok {
		return icalDuration{}, invalidDuration(value)
	}

	datePart, timePart, hasTime := strings.Cut(rest, "T")

	duration, err := parseDurationDate(value, datePart)
	if err != nil {
		return icalDuration{}, err
	}

	if hasTime {
		if timePart == "" {
			return icalDuration{}, invalidDuration(value)
		}

		duration.exact, err = parseDurationTime(value, timePart)
		if err != nil {
			return icalDuration{}, err
		}
	}

	if negative {
		duration.days = -duration.days
		duration.exact = -duration.exact
	}

	return duration, nil
}

// parseDurationDate parses the date part, either "nW" alone or "nD".
func parseDurationDate(value, part string) (icalDuration, error) {
	if part == "" {
		return icalDuration{}, nil
	}

	number, suffix, rest, err := scanDurationComponent(value, part)
	if err != nil {
		return icalDuration{}, err
	}

	days, ok := icalDurationDateUnits[suffix]
	if !ok || rest != "" {
		return icalDuration{}, invalidDuration(value)
	}

	return icalDuration{days: number * days}, nil
}

// parseDurationTime parses the time part as "nH", "nM" and "nS" in that order.
func parseDurationTime(value, part string) (time.Duration, error) {
	var (
		exact time.Duration
		last  = -1
	)

	for part != "" {
		number, suffix, rest, err := scanDurationComponent(value, part)
		if err != nil {
			return 0, err
		}

		unit, ok := icalDurationTimeUnits[suffix]
		if !ok {
			return 0, invalidDuration(value)
		}

		index := strings.IndexByte("HMS", suffix)
		if index <= last {
			return 0, invalidDuration(value)
		}

		last = index
		exact += time.Duration(number) * unit
		part = rest
	}

	return exact, nil
}

// scanDurationComponent reads one number and unit suffix from part.
func scanDurationComponent(value, part string) (int, byte, string, error) {
	digits := strings.IndexFunc(part, func(r rune) bool {
		return r < '0' || r > '9'
	})

	// A component needs digits and a unit suffix.
	if digits <= 0 || digits == len(part) {
		return 0, 0, "", invalidDuration(value)
	}

	number, err := strconv.Atoi(part[:digits])
	if err != nil {
		return 0, 0, "", invalidDuration(value)
	}

	return number, part[digits], part[digits+1:], nil
}

func invalidDuration(value string) error {
	return fmt.Errorf("%w: %q", ErrInvalidDuration, value)
}
