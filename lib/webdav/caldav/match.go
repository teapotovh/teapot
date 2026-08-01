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
	zeroDate            time.Time
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
		ok, err := Match(*query, &co)
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

// Match reports whether the provided CalendarObject matches the query.
func Match(query CompFilter, co *CalendarObject) (bool, error) {
	if co.Data == nil || len(co.Data.Components) <= 0 {
		return false, ErrMatchEmptyObject
	}

	// TODO handle more than just events
	for i, event := range co.Data.Events() {
		matches, err := match(query, event)
		if err != nil {
			return false, fmt.Errorf("parsing component %d: %w", i, err)
		}

		if matches {
			return true, nil
		}
	}

	return false, nil
}

func match(filter CompFilter, comp *ics.VEvent) (bool, error) {
	// TODO: can we support this? doesn't look like Components have a NAME property
	// if comp.Name != filter.Name {
	// 	return filter.IsNotDefined, nil
	// }
	if filter.Start != zeroDate {
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
			match, err := match(filter, event)
			if err != nil {
				return false, err
			} else {
				matches = matches || match
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
	case filter.Start != zeroDate:
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

//nolint:gocyclo
func matchCompTimeRange(start, end time.Time, comp *ics.VEvent) (bool, error) {
	// See https://datatracker.ietf.org/doc/html/rfc4791#section-9.9
	eventStart, err := comp.GetStartAt()
	if err != nil {
		return false, fmt.Errorf("while parsing event start time: %w", err)
	}

	rules, err := comp.GetRRules()
	if err != nil {
		return false, fmt.Errorf("while parsing event recurrence rules: %w", err)
	}

	switch {
	case len(rules) > 0:
		// Build an RRuleSet from the event's recurrence properties
		rset := &rrule.Set{}

		for _, r := range rules {
			rr, _ := rrule.StrToRRule(r.String())
			rr.DTStart(eventStart)
			rset.RRule(rr)
		}

		rDates, err := comp.GetRDates()
		if err != nil {
			return false, fmt.Errorf("while parsing event recurrence dates: %w", err)
		}

		for _, rd := range rDates {
			rset.RDate(rd)
		}

		exDates, err := comp.GetExDates()
		if err != nil {
			return false, fmt.Errorf("while parsing event exclude dates: %w", err)
		}

		for _, exd := range exDates {
			rset.ExDate(exd)
		}

		// TODO we can only set inclusive to true or false, but really the
		// start time is inclusive while the end time is not :/
		return len(rset.Between(start, end, true)) > 0, nil

	default:
		eventEnd, err := comp.GetEndAt()
		if err != nil {
			return false, fmt.Errorf("while parsing event end time: %w", err)
		}

		// Event starts in time range
		if eventStart.After(start) && (end.IsZero() || eventStart.Before(end)) {
			return true, nil
		}
		// Event ends in time range
		if eventEnd.After(start) && (end.IsZero() || eventEnd.Before(end)) {
			return true, nil
		}
		// Event covers entire time range plus some
		if eventStart.Before(start) && (!end.IsZero() && eventEnd.After(end)) {
			return true, nil
		}

		return false, nil
	}
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
