package caldav

import (
	"errors"
	"fmt"
	"strings"
	"time"

	ics "github.com/arran4/golang-ical"
	"github.com/teambition/rrule-go"
)

var (
	ErrMatchEmptyObject = errors.New("request to process empty calendar object")
	ErrInvalidTopLevel  = errors.New("invalid top level filter")
)

// Filter returns the filtered list of calendar objects matching the provided query.
// A nil query will return the full list of calendar objects.
func Filter(query *CompFilter, cos []CalendarObject) ([]CalendarObject, error) {
	if query == nil {
		// FIXME: should we always return a copy of the provided slice?
		return cos, nil
	}

	var out []CalendarObject

	for _, co := range cos {
		ok, err := MatchCalendar(*query, co.Data)
		if err != nil {
			return nil, fmt.Errorf("error while matching object at %q: %w", co.Path, err)
		}

		if !ok {
			continue
		}

		// TODO properties are not currently filtered even if requested
		out = append(out, co)
	}

	return out, nil
}

// MatchCalendar reports whether the provided [*ics.Calendar] matches the query.
func MatchCalendar(query CompFilter, cal *ics.Calendar) (bool, error) {
	if cal == nil || len(cal.Components) <= 0 {
		return false, ErrMatchEmptyObject
	}

	if query.Name != string(ics.ComponentVCalendar) {
		// Filter-in unsupported component types
		return true, fmt.Errorf("could not apply filter %q: %w", query.Name, ErrInvalidTopLevel)
	}

	// TODO checks other properties of VCALENDAR component
	// TODO checks components != VEVENT and VTODO too

	// RFC 4791, section 9.7.1 requires every component filter to match.
	for _, childFilter := range query.Comps {
		childMatches, err := matchCalendarChild(childFilter, cal)
		if err != nil {
			return false, err
		}

		if !childMatches {
			return false, nil
		}
	}

	return len(query.Comps) > 0, nil
}

// matchCalendarChild reports whether a child of the VCALENDAR filter matches the calendar.
func matchCalendarChild(filter CompFilter, cal *ics.Calendar) (bool, error) {
	switch {
	case strings.EqualFold(filter.Name, string(ics.ComponentVEvent)):
		for _, child := range cal.Events() {
			childMatches, err := matchEvent(filter, child)
			if err != nil {
				return false, fmt.Errorf("matching event component: %w", err)
			}

			if childMatches {
				return true, nil
			}
		}

	case strings.EqualFold(filter.Name, string(ics.ComponentVTodo)):
		for _, child := range cal.Todos() {
			childMatches, err := matchTodo(filter, child)
			if err != nil {
				return false, fmt.Errorf("matching todo component: %w", err)
			}

			if childMatches {
				return true, nil
			}
		}

	default:
		// Filter-in unsupported component types
		return true, nil
	}

	return false, nil
}

// componentMatcher matches a filter against one calendar component.
type componentMatcher interface {
	match(filter CompFilter) (bool, error)
}

func matchComponentFilters(
	filter CompFilter,
	comp *ics.ComponentBase,
	alarmStart, alarmEnd time.Time,
) (bool, error) {
	for _, compFilter := range filter.Comps {
		match, err := matchCompFilter(compFilter, comp, alarmStart, alarmEnd)
		if err != nil {
			return false, fmt.Errorf("error while matching component filter: %w", err)
		}

		if !match {
			return false, nil
		}
	}

	for _, propFilter := range filter.Props {
		match, err := matchPropFilter(propFilter, comp)
		if err != nil {
			return false, fmt.Errorf("error while matching filter props: %w", err)
		}

		if !match {
			return false, nil
		}
	}

	return true, nil
}

func matchCompFilter(
	filter CompFilter,
	comp *ics.ComponentBase,
	alarmStart, alarmEnd time.Time,
) (bool, error) {
	for _, child := range comp.Components {
		var (
			matcher componentMatcher
			err     error
		)

		switch child := child.(type) {
		case *ics.VEvent:
			matcher, err = newEventMatcher(child)

		case *ics.VTodo:
			matcher, err = newTodoMatcher(child)

		case *ics.VAlarm:
			matcher = newAlarmMatcher(child, alarmStart, alarmEnd)

		default:
			continue
		}

		if err != nil {
			return false, err
		}

		match, err := matcher.match(filter)
		if err != nil {
			return false, err
		}

		if match {
			return true, nil
		}
	}

	return filter.IsNotDefined, nil
}

func matchPropFilter(filter PropFilter, comp *ics.ComponentBase) (bool, error) {
	prop := ics.ComponentProperty(filter.Name)
	properties := comp.GetProperties(prop)

	if len(properties) == 0 {
		return filter.IsNotDefined, nil
	}

	if filter.IsNotDefined {
		return false, nil
	}

	if !filter.Start.IsZero() {
		return matchPropDateFilter(filter, comp, prop)
	}

	// A prop-filter matches if any of the property values satisfies all the
	// parameter filters and the text-match.
	for _, field := range properties {
		if matchPropValue(filter, field) {
			return true, nil
		}
	}

	return false, nil
}

func matchPropValue(filter PropFilter, field *ics.IANAProperty) bool {
	for _, paramFilter := range filter.ParamFilter {
		if !matchParamFilter(paramFilter, field) {
			return false
		}
	}

	return filter.TextMatch == nil || matchTextMatch(*filter.TextMatch, field.Value)
}

func matchPropDateFilter(
	filter PropFilter,
	comp *ics.ComponentBase,
	prop ics.ComponentProperty,
) (bool, error) {
	var (
		dates []time.Time
		err   error
	)

	switch prop { //nolint:exhaustive
	case ics.ComponentPropertyRdate:
		dates, err = comp.GetRDates()

	case ics.ComponentPropertyExdate:
		dates, err = comp.GetExDates()

	default:
		// Matching a date against a non-date prop, invalid
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("error while getting dates for property %q: %w", string(prop), err)
	}

	// RFC 4791, section 9.7.2 matches if a property value is in the range.
	for _, date := range dates {
		if matchPropTimeRange(filter.Start, filter.End, date) {
			return true, nil
		}
	}

	return false, nil
}

// recurrenceStarts returns the recurrence instances to test for a component.
// It returns the seed when the component has no recurrence rules.
func recurrenceStarts(
	comp *ics.ComponentBase,
	seed time.Time,
	duration time.Duration,
	start, end time.Time,
) ([]time.Time, error) {
	rules, err := comp.GetRRules()
	if err != nil {
		return nil, fmt.Errorf("while parsing recurrence rules: %w", err)
	}

	rDates, err := comp.GetRDates()
	if err != nil {
		return nil, fmt.Errorf("while parsing recurrence dates: %w", err)
	}

	exDates, err := comp.GetExDates()
	if err != nil {
		return nil, fmt.Errorf("while parsing exclude dates: %w", err)
	}

	if len(rules) == 0 && len(rDates) == 0 && len(exDates) == 0 {
		return []time.Time{seed}, nil
	}

	rset := &rrule.Set{}

	for _, r := range rules {
		rr, err := rrule.StrToRRule(r.String())
		if err != nil {
			return nil, fmt.Errorf("while parsing recurrence rule: %w", err)
		}

		rr.DTStart(seed)
		rset.RRule(rr)
	}

	for _, rd := range rDates {
		rset.RDate(rd)
	}

	for _, exd := range exDates {
		rset.ExDate(exd)
	}

	// An occurrence can overlap the range even if its start is before
	// the range start, so expand the search backwards by the duration.
	searchStart := start.Add(-duration)

	if end.IsZero() {
		next := rset.Iterator()

		for {
			t, ok := next()
			if !ok {
				return nil, nil
			}

			if !t.Before(searchStart) {
				return []time.Time{t}, nil
			}
		}
	}

	return rset.Between(searchStart, end, true), nil
}

func matchPropTimeRange(start, end, ptime time.Time) bool {
	if ptime.After(start) && (end.IsZero() || ptime.Before(end)) {
		return true
	}

	return false
}

func matchParamFilter(filter ParamFilter, field *ics.IANAProperty) bool {
	values, exists := field.ICalParameters[filter.Name]
	if !exists {
		return filter.IsNotDefined
	} else if len(values) > 0 && filter.IsNotDefined {
		return false
	}

	for _, value := range values {
		if filter.TextMatch != nil {
			return matchTextMatch(*filter.TextMatch, value)
		}
	}

	return true
}

func matchTextMatch(txt TextMatch, value string) bool {
	// TODO: handle text-match collation attribute
	match := strings.Contains(value, txt.Text)
	if txt.NegateCondition {
		match = !match
	}

	return match
}
