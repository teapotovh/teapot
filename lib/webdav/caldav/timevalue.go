package caldav

import (
	"fmt"
	"strings"
	"time"

	ics "github.com/arran4/golang-ical"
)

const (
	dateLayout     = "20060102"
	dateTimeLayout = "20060102T150405"
)

// parseTimeValue parses one iCalendar DATE or DATE-TIME value. Floating values
// use defaultLocation, and a nil defaultLocation means UTC.
func parseTimeValue(
	value string,
	params map[string][]string,
	defaultLocation *time.Location,
) (time.Time, error) {
	if defaultLocation == nil {
		defaultLocation = time.UTC
	}

	value = strings.TrimSpace(value)

	if isDateValue(value) {
		return time.ParseInLocation(dateLayout, value, defaultLocation)
	}

	if strings.HasSuffix(value, "Z") {
		return time.Parse(dateWithUTCTimeLayout, value)
	}

	location := defaultLocation

	if tzids := params[string(ics.ParameterTzid)]; len(tzids) > 0 {
		tzid := unquoteValue(tzids[0])

		loc, err := time.LoadLocation(tzid)
		if err != nil {
			return time.Time{}, fmt.Errorf("while loading timezone %q: %w", tzid, err)
		}

		location = loc
	}

	return time.ParseInLocation(dateTimeLayout, value, location)
}

// isDateValue reports whether the value is an iCalendar DATE. A DATE value has
// no time separator, so VALUE=DATE needs no separate check.
func isDateValue(value string) bool {
	return !strings.Contains(value, "T")
}

func unquoteValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) {
		return value[1 : len(value)-1]
	}

	return value
}

// parseTimeProperty parses a time property using the default location for
// floating values.
func parseTimeProperty(
	comp *ics.ComponentBase,
	prop ics.ComponentProperty,
	defaultLocation *time.Location,
) (time.Time, error) {
	property := comp.GetProperty(prop)
	if property == nil {
		return time.Time{}, fmt.Errorf("%w: %s", ics.ErrPropertyNotFound, prop)
	}

	parsed, err := parseTimeValue(property.Value, property.ICalParameters, defaultLocation)
	if err != nil {
		return time.Time{}, fmt.Errorf("while parsing property %q: %w", string(prop), err)
	}

	return parsed, nil
}

// multiTimeProperty parses all values of a multi-value time property, such as
// RDATE or EXDATE, using the default location for floating values.
func multiTimeProperty(
	comp *ics.ComponentBase,
	prop ics.ComponentProperty,
	defaultLocation *time.Location,
) ([]time.Time, error) {
	var times []time.Time

	for _, property := range comp.GetProperties(prop) {
		for value := range strings.SplitSeq(property.Value, ",") {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}

			parsed, err := parseTimeValue(value, property.ICalParameters, defaultLocation)
			if err != nil {
				return nil, fmt.Errorf("while parsing property %q: %w", string(prop), err)
			}

			times = append(times, parsed)
		}
	}

	return times, nil
}
