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

	for _, childFilter := range query.Comps {
		switch {
		case strings.EqualFold(childFilter.Name, string(ics.ComponentVEvent)):
			for _, child := range cal.Events() {
				childMatches, err := matchEvent(childFilter, child)
				if err != nil {
					return false, fmt.Errorf("matching event component: %w", err)
				}

				if childMatches {
					return true, nil // early exit if one of the children matches
				}
			}

		case strings.EqualFold(childFilter.Name, string(ics.ComponentVTodo)):
			for _, child := range cal.Todos() {
				childMatches, err := matchTodo(childFilter, child)
				if err != nil {
					return false, fmt.Errorf("matching todo component: %w", err)
				}

				if childMatches {
					return true, nil // early exit if one of the children matches
				}
			}

		default:
			// Filter-in unsupported component types
			return true, nil
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

	if !filter.Start.IsZero() || !filter.End.IsZero() {
		match, err := matchCompTimeRange(filter.Start, filter.End, comp)
		if err != nil {
			return false, fmt.Errorf("error while matching time: %w", err)
		}

		if !match {
			return false, nil
		}
	}

	return matchComponentFilters(filter, &comp.ComponentBase)
}

// matchTodo matches a CompFilter against a VTODO component. It returns true if the component matches the filter,
// false otherwise.
func matchTodo(filter CompFilter, comp *ics.VTodo) (bool, error) {
	if !strings.EqualFold(string(ics.ComponentVTodo), filter.Name) {
		return false, nil
	}

	if !filter.Start.IsZero() || !filter.End.IsZero() {
		match, err := matchTodoTimeRange(filter.Start, filter.End, comp)
		if err != nil {
			return false, fmt.Errorf("error while matching time: %w", err)
		}

		if !match {
			return false, nil
		}
	}

	return matchComponentFilters(filter, &comp.ComponentBase)
}

func matchComponentFilters(filter CompFilter, comp *ics.ComponentBase) (bool, error) {
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

func matchCompFilter(filter CompFilter, comp *ics.ComponentBase) (bool, error) {
	for _, child := range comp.Components {
		var match bool

		switch child := child.(type) {
		case *ics.VEvent:
			if !strings.EqualFold(filter.Name, string(ics.ComponentVEvent)) {
				continue
			}

			var err error

			match, err = matchEvent(filter, child)
			if err != nil {
				return false, err
			}

		case *ics.VTodo:
			if !strings.EqualFold(filter.Name, string(ics.ComponentVTodo)) {
				continue
			}

			var err error

			match, err = matchTodo(filter, child)
			if err != nil {
				return false, err
			}

		default:
			continue
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

// intervalsOverlap reports whether the event interval overlaps the time
// range. A point event (no DTEND and no DURATION) matches when its instant is
// within the range. An event with an explicit end uses the overlap formula
// from RFC 4791, section 9.9.
func intervalsOverlap(
	eventStart, eventEnd,
	rangeStart, rangeEnd time.Time,
	pointEvent bool,
) bool {
	if pointEvent {
		// The range start is inclusive and the range end is exclusive.
		return !eventStart.Before(rangeStart) && (rangeEnd.IsZero() || eventStart.Before(rangeEnd))
	}

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

	eventEndProp := comp.GetProperty(ics.ComponentPropertyDtEnd)
	durationProp := comp.GetProperty(ics.ComponentPropertyDuration)

	// A point event has neither an explicit end nor a duration. Only such an
	// event uses the inclusive-start point semantics.
	pointEvent := eventEndProp == nil && durationProp == nil

	var eventEnd time.Time

	switch {
	case eventEndProp != nil:
		eventEnd, err = comp.GetEndAt()
		if err != nil {
			return false, fmt.Errorf("while parsing event end time: %w", err)
		}

	case durationProp != nil:
		eventEnd, err = endFromDuration(&comp.ComponentBase, eventStart)
		if err != nil {
			return false, err
		}

	default:
		eventEnd = eventStart
	}

	duration := eventEnd.Sub(eventStart)

	occurrences, err := recurrenceStarts(&comp.ComponentBase, eventStart, duration, start, end)
	if err != nil {
		return false, err
	}

	for _, occurrenceStart := range occurrences {
		occurrenceEnd := occurrenceStart.Add(duration)

		if intervalsOverlap(
			occurrenceStart,
			occurrenceEnd,
			start,
			end,
			pointEvent,
		) {
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

type todoState struct {
	start, due, completed, created time.Time
	duration                       time.Duration
	hasStart, hasDue               bool
	hasDuration                    bool
	hasCompleted, hasCreated       bool
}

// matchTodoTimeRange reports whether a VTODO overlaps the time range, as
// defined in RFC 4791, section 9.9.
func matchTodoTimeRange(start, end time.Time, comp *ics.VTodo) (bool, error) {
	state, err := newTodoState(comp)
	if err != nil {
		return false, err
	}

	if !state.hasStart && !state.hasDue {
		return state.matches(start, end), nil
	}

	seed := state.start
	if !state.hasStart {
		seed = state.due
	}

	occurrences, err := recurrenceStarts(&comp.ComponentBase, seed, state.duration, start, end)
	if err != nil {
		return false, err
	}

	for _, occurrence := range occurrences {
		if state.at(occurrence).matches(start, end) {
			return true, nil
		}
	}

	return false, nil
}

func newTodoState(comp *ics.VTodo) (todoState, error) {
	base := &comp.ComponentBase

	state := todoState{
		hasStart:     base.GetProperty(ics.ComponentPropertyDtStart) != nil,
		hasDue:       base.GetProperty(ics.ComponentPropertyDue) != nil,
		hasDuration:  base.GetProperty(ics.ComponentPropertyDuration) != nil,
		hasCompleted: base.GetProperty(ics.ComponentPropertyCompleted) != nil,
		hasCreated:   base.GetProperty(ics.ComponentPropertyCreated) != nil,
	}

	var err error

	if state.hasStart {
		state.start, err = comp.GetStartAt()
		if err != nil {
			return todoState{}, fmt.Errorf("while parsing todo start time: %w", err)
		}
	}

	if state.hasDue {
		state.due, err = comp.GetDueAt()
		if err != nil {
			return todoState{}, fmt.Errorf("while parsing todo due time: %w", err)
		}
	}

	if state.hasCompleted {
		state.completed, err = parseUTCTimeProperty(base, ics.ComponentPropertyCompleted)
		if err != nil {
			return todoState{}, fmt.Errorf("while parsing todo completed time: %w", err)
		}
	}

	if state.hasCreated {
		state.created, err = parseUTCTimeProperty(base, ics.ComponentPropertyCreated)
		if err != nil {
			return todoState{}, fmt.Errorf("while parsing todo created time: %w", err)
		}
	}

	switch {
	case state.hasStart && state.hasDuration:
		state.due, err = endFromDuration(base, state.start)
		if err != nil {
			return todoState{}, fmt.Errorf("while computing todo end time: %w", err)
		}

		state.duration = state.due.Sub(state.start)

	case state.hasStart && state.hasDue:
		state.duration = state.due.Sub(state.start)
	}

	return state, nil
}

// at returns the state of a recurrence instance that starts at the provided
// time.
func (s todoState) at(occurrence time.Time) todoState {
	switch {
	case s.hasStart && s.hasDuration:
		s.start = occurrence
		s.due = occurrence.Add(s.duration)

	case s.hasStart && s.hasDue:
		s.start = occurrence
		s.due = occurrence.Add(s.duration)

	case s.hasStart:
		s.start = occurrence

	case s.hasDue:
		s.due = occurrence
	}

	return s
}

// matches applies the VTODO conditions from RFC 4791, section 9.9.
func (s todoState) matches(start, end time.Time) bool {
	from, to, includeFrom, includeTo, defined := s.interval()
	if !defined {
		return true
	}

	return rangeOverlaps(start, end, from, to, includeFrom, includeTo)
}

// interval returns the effective interval of the VTODO. includeFrom lets the
// interval start equal the range end, and includeTo lets it end at the range
// start. A zero end means an open interval.
func (s todoState) interval() (from, to time.Time, includeFrom, includeTo, defined bool) {
	switch {
	case s.hasStart && s.hasDuration:
		to = s.start.Add(s.duration)
		return s.start, to, s.duration == 0, true, true

	case s.hasStart && s.hasDue:
		point := s.start.Equal(s.due)
		return s.start, s.due, point, point, true

	case s.hasStart:
		return s.start, s.start, false, true, true

	case s.hasDue:
		return s.due, s.due, true, false, true

	case s.hasCompleted && s.hasCreated:
		from, to = s.created, s.completed
		if to.Before(from) {
			from, to = to, from
		}

		return from, to, true, true, true

	case s.hasCompleted:
		return s.completed, s.completed, true, true, true

	case s.hasCreated:
		return s.created, time.Time{}, false, false, true

	default:
		return time.Time{}, time.Time{}, false, false, false
	}
}

// rangeOverlaps reports whether the time range overlaps the interval. A zero
// range bound is open. The inclusive flags control the interval boundaries.
func rangeOverlaps(
	rangeStart, rangeEnd,
	from, to time.Time,
	includeFrom, includeTo bool,
) bool {
	if !rangeStart.IsZero() && !to.IsZero() {
		if includeTo && to.Before(rangeStart) {
			return false
		}

		if !includeTo && !to.After(rangeStart) {
			return false
		}
	}

	if !rangeEnd.IsZero() {
		if includeFrom && from.After(rangeEnd) {
			return false
		}

		if !includeFrom && !from.Before(rangeEnd) {
			return false
		}
	}

	return true
}

func parseUTCTimeProperty(comp *ics.ComponentBase, prop ics.ComponentProperty) (time.Time, error) {
	property := comp.GetProperty(prop)
	if property == nil {
		return time.Time{}, fmt.Errorf("%w: %s", ics.ErrPropertyNotFound, prop)
	}

	parsed, err := time.Parse(dateWithUTCTimeLayout, property.Value)
	if err != nil {
		return time.Time{}, fmt.Errorf("while parsing property %q: %w", string(prop), err)
	}

	return parsed, nil
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
