package caldav

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	ics "github.com/arran4/golang-ical"
)

// alarmMatcher matches a filter against a VALARM component.
type alarmMatcher struct {
	comp                   *ics.VAlarm
	parentStart, parentEnd time.Time
}

func newAlarmMatcher(
	comp *ics.VAlarm,
	parentStart, parentEnd time.Time,
) componentMatcher {
	return alarmMatcher{
		comp:        comp,
		parentStart: parentStart,
		parentEnd:   parentEnd,
	}
}

func (m alarmMatcher) match(filter CompFilter) (bool, error) {
	if !strings.EqualFold(string(ics.ComponentVAlarm), filter.Name) {
		return false, nil
	}

	if !filter.Start.IsZero() || !filter.End.IsZero() {
		match, err := matchAlarmTimeRange(filter.Start, filter.End, m.comp, m.parentStart, m.parentEnd)
		if err != nil {
			return false, fmt.Errorf("error while matching time: %w", err)
		}

		if !match {
			return false, nil
		}
	}

	return matchComponentFilters(filter, &m.comp.ComponentBase, time.Time{}, time.Time{})
}

func matchAlarmTimeRange(
	start, end time.Time,
	comp *ics.VAlarm,
	parentStart, parentEnd time.Time,
) (bool, error) {
	prop := comp.GetProperty(ics.ComponentPropertyTrigger)
	if prop == nil {
		return false, fmt.Errorf("%w: TRIGGER", ics.ErrPropertyNotFound)
	}

	reference := parentStart
	if values := prop.ICalParameters[string(ics.ParameterRelated)]; len(values) > 0 &&
		strings.EqualFold(values[0], "END") {
		reference = parentEnd
	}

	trigger, err := alarmTrigger(prop, reference)
	if err != nil {
		return false, err
	}

	repeats, interval, err := alarmRepeat(comp)
	if err != nil {
		return false, err
	}

	for range repeats + 1 {
		if timeInRange(start, end, trigger) {
			return true, nil
		}

		trigger = interval.addTo(trigger)
	}

	return false, nil
}

func alarmTrigger(prop *ics.IANAProperty, reference time.Time) (time.Time, error) {
	if values := prop.ICalParameters[string(ics.ParameterValue)]; len(values) > 0 &&
		strings.EqualFold(values[0], string(ics.ValueDataTypeDateTime)) {
		trigger, err := time.Parse(dateWithUTCTimeLayout, prop.Value)
		if err != nil {
			return time.Time{}, fmt.Errorf("while parsing alarm trigger: %w", err)
		}

		return trigger, nil
	}

	offset, err := parseICalDuration(prop.Value)
	if err != nil {
		return time.Time{}, fmt.Errorf("while parsing alarm trigger: %w", err)
	}

	return offset.addTo(reference), nil
}

func alarmRepeat(comp *ics.VAlarm) (int, icalDuration, error) {
	prop := comp.GetProperty(ics.ComponentProperty(ics.PropertyRepeat))
	if prop == nil {
		return 0, icalDuration{}, nil
	}

	repeats, err := strconv.Atoi(prop.Value)
	if err != nil {
		return 0, icalDuration{}, fmt.Errorf("while parsing alarm repeat: %w", err)
	}

	if repeats <= 0 {
		return 0, icalDuration{}, nil
	}

	durationProp := comp.GetProperty(ics.ComponentPropertyDuration)
	if durationProp == nil {
		return 0, icalDuration{}, fmt.Errorf("%w: DURATION", ics.ErrPropertyNotFound)
	}

	interval, err := parseICalDuration(durationProp.Value)
	if err != nil {
		return 0, icalDuration{}, fmt.Errorf("while parsing alarm duration: %w", err)
	}

	return repeats, interval, nil
}

func timeInRange(start, end, t time.Time) bool {
	return (start.IsZero() || !t.Before(start)) && (end.IsZero() || t.Before(end))
}
