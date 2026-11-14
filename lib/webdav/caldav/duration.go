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

	if duration.negative() {
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

func (d icalDuration) negative() bool {
	return d.days < 0 || d.exact < 0
}

func (d icalDuration) addTo(t time.Time) time.Time {
	return t.AddDate(0, 0, d.days).Add(d.exact)
}

// icalDurationUnits maps a unit suffix to its value. Day units are nominal,
// time units are exact.
var icalDurationUnits = map[byte]icalDuration{
	'W': {days: 7},
	'D': {days: 1},
	'H': {exact: time.Hour},
	'M': {exact: time.Minute},
	'S': {exact: time.Second},
}

// parseICalDuration parses a duration value as defined in RFC 5545, section
// 3.3.6. The input is case-insensitive. A bare "P" is accepted for parity with
// the previous parser.
func parseICalDuration(value string) (icalDuration, error) {
	rest, negative := splitDurationSign(strings.ToUpper(value))

	if !strings.HasPrefix(rest, "P") {
		return icalDuration{}, invalidDuration(value)
	}

	duration, err := scanDurationParts(value, rest[1:])
	if err != nil {
		return icalDuration{}, err
	}

	if negative {
		duration.days = -duration.days
		duration.exact = -duration.exact
	}

	return duration, nil
}

// splitDurationSign removes an optional leading sign and reports whether the
// duration is negative.
func splitDurationSign(value string) (string, bool) {
	switch {
	case strings.HasPrefix(value, "+"):
		return value[1:], false
	case strings.HasPrefix(value, "-"):
		return value[1:], true
	default:
		return value, false
	}
}

// scanDurationParts scans the part after "P". Day units are only valid before
// T, time units only after T, and weeks can not be combined with other units.
func scanDurationParts(value, rest string) (icalDuration, error) {
	var (
		duration  icalDuration
		timeMode  bool
		weeksOnly bool
	)

	for rest != "" {
		if rest[0] == 'T' {
			if timeMode || len(rest) == 1 {
				return icalDuration{}, invalidDuration(value)
			}

			timeMode = true
			rest = rest[1:]

			continue
		}

		number, suffix, tail, err := scanDurationComponent(value, rest)
		if err != nil {
			return icalDuration{}, err
		}

		rest = tail

		unit, ok := icalDurationUnits[suffix]
		if !ok || (unit.days > 0) == timeMode {
			return icalDuration{}, invalidDuration(value)
		}

		if suffix == 'W' {
			if weeksOnly || duration.days != 0 || duration.exact != 0 {
				return icalDuration{}, invalidDuration(value)
			}

			weeksOnly = true
		} else if weeksOnly {
			return icalDuration{}, invalidDuration(value)
		}

		if unit.days > 0 {
			duration.days += number * unit.days
		} else {
			duration.exact += time.Duration(number) * unit.exact
		}
	}

	return duration, nil
}

// scanDurationComponent reads one number and unit suffix from rest.
func scanDurationComponent(value, rest string) (int, byte, string, error) {
	digits := 0
	for digits < len(rest) && rest[digits] >= '0' && rest[digits] <= '9' {
		digits++
	}

	// A component needs digits and a unit suffix.
	if digits == 0 || digits == len(rest) {
		return 0, 0, "", invalidDuration(value)
	}

	number, err := strconv.Atoi(rest[:digits])
	if err != nil {
		return 0, 0, "", invalidDuration(value)
	}

	return number, rest[digits], rest[digits+1:], nil
}

func invalidDuration(value string) error {
	return fmt.Errorf("%w: %q", ErrInvalidDuration, value)
}
