package caldav

import (
	"fmt"
	"strings"
	"time"

	ics "github.com/arran4/golang-ical"
)

// todoMatcher matches a filter against a VTODO component.
type todoMatcher struct {
	comp  *ics.VTodo
	state todoState
}

func newTodoMatcher(comp *ics.VTodo) (componentMatcher, error) {
	state, err := newTodoState(comp)
	if err != nil {
		return nil, err
	}

	return todoMatcher{comp: comp, state: state}, nil
}

func (m todoMatcher) match(filter CompFilter) (bool, error) {
	if !strings.EqualFold(string(ics.ComponentVTodo), filter.Name) {
		return false, nil
	}

	if !filter.Start.IsZero() || !filter.End.IsZero() {
		match, err := matchTodoTimeRange(filter.Start, filter.End, m.comp, m.state)
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
		m.state.alarmStart(),
		m.state.alarmEnd(),
	)
}

func matchTodo(filter CompFilter, comp *ics.VTodo) (bool, error) {
	matcher, err := newTodoMatcher(comp)
	if err != nil {
		return false, fmt.Errorf("while parsing todo: %w", err)
	}

	return matcher.match(filter)
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
func matchTodoTimeRange(start, end time.Time, comp *ics.VTodo, state todoState) (bool, error) {
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

// alarmStart returns the reference start time for the alarms of the todo.
func (s todoState) alarmStart() time.Time {
	if s.hasStart {
		return s.start
	}

	return s.due
}

// alarmEnd returns the reference end time for the alarms of the todo.
func (s todoState) alarmEnd() time.Time {
	if !s.due.IsZero() {
		return s.due
	}

	return s.alarmStart()
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
