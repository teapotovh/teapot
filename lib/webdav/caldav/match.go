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

// MatchCalendar reports whether the provided CalendarObject matches the query.
func MatchCalendar(query CompFilter, cal *ics.Calendar) (bool, error) {
	if len(cal.Components) == 0 {
		return false, ErrMatchEmptyObject
	}

	if query.Name != string(ics.ComponentVCalendar) {
		return false, nil
	}

	// TODO checks other properties of VCALENDAR component
	// TODO checks components != VEVENT too

	for _, child := range cal.Events() {
		childMatches := false

		for _, childFilter := range query.Comps {
			m, err := matchEvent(childFilter, child)
			if err != nil {
				return false, fmt.Errorf("matching children component: %w", err)
			}

			childMatches = childMatches || m
			if childMatches {
				return true, nil // early exit if one of the children matches
			}
		}
	}

	return false, nil
}

// matchEvent matches a CompFilter against a VEvent component. It returns true if the component matches the filter,
// false otherwise.
func matchEvent(filter CompFilter, comp *ics.VEvent) (bool, error) {
	if !strings.EqualFold(string(ics.ComponentVEvent), filter.Name) {
		return false, nil
	}

	if !filter.Start.IsZero() {
		match, err := matchCompTimeRange(filter.Start, filter.End, comp)
		if err != nil {
			return false, fmt.Errorf("error while matching time: %w", err)
		}

		if !match {
			return false, nil
		}

	}

	for _, compFilter := range filter.Comps {
		match, err := matchCompFilter(compFilter, comp)
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

func matchCompFilter(filter CompFilter, comp *ics.VEvent) (bool, error) {
	matches := false

	for _, child := range comp.Components {
		switch event := child.(type) {
		case *ics.VEvent:
			match, err := matchEvent(filter, event)
			if err != nil {
				return false, err
			}

			matches = matches || match
			if matches {
				break // early exit
			}
		}
	}

	if !matches {
		return filter.IsNotDefined, nil
	}

	return true, nil
}

func matchPropFilter(filter PropFilter, comp *ics.VEvent) (bool, error) {
	prop := ics.ComponentProperty(filter.Name)
	if prop.Singular(comp) {
		field := comp.GetProperty(prop)
		if field == nil {
			return filter.IsNotDefined, nil
		}
	}

	for _, field := range comp.GetProperties(prop) {
		for _, paramFilter := range filter.ParamFilter {
			if !matchParamFilter(paramFilter, field) {
				return false, nil
			}
		}
	}

	// We support two types of matches:
	// 1. Date matching
	// 2. Text matching
	switch {
	case !filter.Start.IsZero():
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

		for _, date := range dates {
			if !matchPropTimeRange(filter.Start, filter.End, date) {
				return false, nil
			}
		}

		// All dates match
		return true, nil

	case filter.TextMatch != nil:
		for _, field := range comp.GetProperties(prop) {
			if !matchTextMatch(*filter.TextMatch, field.Value) {
				return false, nil
			}
		}

		// All texts match
		return true, nil
	}

	// empty (or unsupported) prop-filter, property exists
	return true, nil
}

func intervalsOverlap(
	eventStart, eventEnd,
	rangeStart, rangeEnd time.Time,
) bool {
	if !eventEnd.After(rangeStart) {
		return false
	}

	return rangeEnd.IsZero() || eventStart.Before(rangeEnd)
}

func matchCompTimeRange(start, end time.Time, comp *ics.VEvent) (bool, error) {
	eventStart, err := comp.GetStartAt()
	if err != nil {
		return false, fmt.Errorf("while parsing event start time: %w", err)
	}

	eventEnd, err := comp.GetEndAt()
	if err != nil {
		return false, fmt.Errorf("while parsing event end time: %w", err)
	}

	rules, err := comp.GetRRules()
	if err != nil {
		return false, fmt.Errorf("while parsing event recurrence rules: %w", err)
	}

	rDates, err := comp.GetRDates()
	if err != nil {
		return false, fmt.Errorf("while parsing event recurrence dates: %w", err)
	}

	exDates, err := comp.GetExDates()
	if err != nil {
		return false, fmt.Errorf("while parsing event exclude dates: %w", err)
	}

	hasRecurrence := len(rules) > 0 || len(rDates) > 0 || len(exDates) > 0

	if !hasRecurrence {
		return intervalsOverlap(eventStart, eventEnd, start, end), nil
	}

	rset := &rrule.Set{}

	for _, r := range rules {
		rr, err := rrule.StrToRRule(r.String())
		if err != nil {
			return false, fmt.Errorf("while parsing event recurrence rule: %w", err)
		}

		rr.DTStart(eventStart)
		rset.RRule(rr)
	}

	for _, rd := range rDates {
		rset.RDate(rd)
	}

	for _, exd := range exDates {
		rset.ExDate(exd)
	}

	duration := eventEnd.Sub(eventStart)

	// An occurrence can overlap the range even if its start is before
	// the range start, so expand the search backwards by the event
	// duration.
	searchStart := start.Add(-duration)

	var occurrences []time.Time
	if end.IsZero() {
		next := rset.Iterator()

		for {
			t, ok := next()
			if !ok {
				break
			}
			if !t.Before(searchStart) {
				occurrences = append(occurrences, t)
				break
			}
		}
	} else {
		occurrences = rset.Between(searchStart, end, true)
	}

	for _, occurrenceStart := range occurrences {
		occurrenceEnd := occurrenceStart.Add(duration)

		if intervalsOverlap(
			occurrenceStart,
			occurrenceEnd,
			start,
			end,
		) {
			return true, nil
		}
	}

	return false, nil
}

func matchPropTimeRange(start, end, ptime time.Time) bool {
	if ptime.After(start) && (end.IsZero() || ptime.Before(end)) {
		return true
	}

	return false
}

//nolint:gocyclo
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
