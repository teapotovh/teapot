package caldav

import (
	"fmt"
	"strings"
	"time"

	ics "github.com/arran4/golang-ical"
)

// eventMatcher matches a filter against a VEVENT component.
type eventMatcher struct {
	comp  *ics.VEvent
	times eventTimes
}

func newEventMatcher(comp *ics.VEvent) (componentMatcher, error) {
	times, err := newEventTimes(comp)
	if err != nil {
		return nil, err
	}

	return eventMatcher{comp: comp, times: times}, nil
}

func (m eventMatcher) match(filter CompFilter) (bool, error) {
	if !strings.EqualFold(string(ics.ComponentVEvent), filter.Name) {
		return false, nil
	}

	if !filter.Start.IsZero() || !filter.End.IsZero() {
		match, err := matchEventTimeRange(filter.Start, filter.End, m.comp, m.times)
		if err != nil {
			return false, fmt.Errorf("error while matching time: %w", err)
		}

		if !match {
			return false, nil
		}
	}

	return matchComponentFilters(filter, &m.comp.ComponentBase, m.times.start, m.times.end)
}

func matchEvent(filter CompFilter, comp *ics.VEvent) (bool, error) {
	matcher, err := newEventMatcher(comp)
	if err != nil {
		return false, fmt.Errorf("while parsing event: %w", err)
	}

	return matcher.match(filter)
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

// eventTimes holds the effective times of a VEVENT component.
type eventTimes struct {
	start, end time.Time
	point      bool
}

func newEventTimes(comp *ics.VEvent) (eventTimes, error) {
	start, err := comp.GetStartAt()
	if err != nil {
		return eventTimes{}, fmt.Errorf("while parsing event start time: %w", err)
	}

	eventEndProp := comp.GetProperty(ics.ComponentPropertyDtEnd)
	durationProp := comp.GetProperty(ics.ComponentPropertyDuration)

	// A point event has neither an explicit end nor a duration. Only such an
	// event uses the inclusive-start point semantics.
	times := eventTimes{
		start: start,
		end:   start,
		point: eventEndProp == nil && durationProp == nil,
	}

	switch {
	case eventEndProp != nil:
		times.end, err = comp.GetEndAt()
		if err != nil {
			return eventTimes{}, fmt.Errorf("while parsing event end time: %w", err)
		}

	case durationProp != nil:
		times.end, err = endFromDuration(&comp.ComponentBase, start)
		if err != nil {
			return eventTimes{}, err
		}
	}

	return times, nil
}

func matchEventTimeRange(start, end time.Time, comp *ics.VEvent, times eventTimes) (bool, error) {
	duration := times.end.Sub(times.start)

	occurrences, err := recurrenceStarts(&comp.ComponentBase, times.start, duration, start, end)
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
			times.point,
		) {
			return true, nil
		}
	}

	return false, nil
}
