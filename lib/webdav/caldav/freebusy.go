package caldav

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	ics "github.com/arran4/golang-ical"
)

const (
	freeBusyContentType = "text/calendar; charset=utf-8"
	freeBusyProductID   = "-//Teapot//CalDAV//EN"

	freeBusyTypeBusy        = "BUSY"
	freeBusyTypeTentative   = "BUSY-TENTATIVE"
	freeBusyTypeUnavailable = "BUSY-UNAVAILABLE"
	statusCancelled         = "CANCELLED"
	statusTentative         = "TENTATIVE"
	transparencyTransparent = "TRANSPARENT"
)

// freeBusyPeriod is a time period during which an event uses the calendar.
type freeBusyPeriod struct {
	Start, End time.Time
}

// computeFreeBusy returns the merged free-busy periods for the calendar
// objects in the time range. Floating values use the provided location.
func computeFreeBusy(
	cos []CalendarObject,
	start, end time.Time,
	location *time.Location,
) (map[string][]freeBusyPeriod, error) {
	result := map[string][]freeBusyPeriod{}

	for _, co := range cos {
		if co.Data == nil {
			continue
		}

		for _, event := range co.Data.Events() {
			fbType, ok := freeBusyEventType(event)
			if !ok {
				continue
			}

			periods, err := freeBusyEventPeriods(event, start, end, location)
			if err != nil {
				return nil, fmt.Errorf("caldav: while computing free-busy for %q: %w", co.Path, err)
			}

			result[fbType] = append(result[fbType], periods...)
		}
	}

	for fbType, periods := range result {
		result[fbType] = mergeFreeBusyPeriods(periods)
	}

	return result, nil
}

func freeBusyEventType(event *ics.VEvent) (string, bool) {
	if transp := event.GetProperty(ics.ComponentPropertyTransp); transp != nil {
		if strings.EqualFold(strings.TrimSpace(transp.Value), transparencyTransparent) {
			return "", false
		}
	}

	if status := event.GetProperty(ics.ComponentPropertyStatus); status != nil {
		switch value := strings.ToUpper(strings.TrimSpace(status.Value)); value {
		case statusCancelled:
			return "", false
		case statusTentative:
			return freeBusyTypeTentative, true
		}
	}

	return freeBusyTypeBusy, true
}

func freeBusyEventPeriods(
	event *ics.VEvent,
	rangeStart, rangeEnd time.Time,
	location *time.Location,
) ([]freeBusyPeriod, error) {
	times, err := freeBusyEventTimes(event, location)
	if err != nil {
		return nil, err
	}

	occurrences, err := recurrenceStarts(
		&event.ComponentBase, times.start, times.duration, rangeStart, rangeEnd, location,
	)
	if err != nil {
		return nil, err
	}

	var periods []freeBusyPeriod

	for _, occurrenceStart := range occurrences {
		occurrenceEnd := occurrenceStart.Add(times.duration)
		if times.allDay {
			occurrenceEnd = occurrenceStart.AddDate(0, 0, times.days)
		}

		if clipped, ok := clipFreeBusyPeriod(occurrenceStart, occurrenceEnd, rangeStart, rangeEnd); ok {
			periods = append(periods, clipped)
		}
	}

	return periods, nil
}

type freeBusyEventInterval struct {
	start    time.Time
	duration time.Duration
	days     int
	allDay   bool
}

func freeBusyEventTimes(event *ics.VEvent, location *time.Location) (freeBusyEventInterval, error) {
	dtstart := event.GetProperty(ics.ComponentPropertyDtStart)
	if dtstart == nil {
		return freeBusyEventInterval{}, fmt.Errorf("%w: %s", ics.ErrPropertyNotFound, ics.ComponentPropertyDtStart)
	}

	times, err := newEventTimes(event, location)
	if err != nil {
		return freeBusyEventInterval{}, err
	}

	result := freeBusyEventInterval{start: times.start, allDay: isDateValue(strings.TrimSpace(dtstart.Value))}

	if !result.allDay {
		result.duration = times.end.Sub(times.start)

		return result, nil
	}

	result.days = 1
	if !times.point {
		result.days = int(times.end.Sub(times.start).Round(24*time.Hour) / (24 * time.Hour))
	}

	result.duration = time.Duration(result.days) * 24 * time.Hour

	return result, nil
}

func clipFreeBusyPeriod(start, end, rangeStart, rangeEnd time.Time) (freeBusyPeriod, bool) {
	if !end.After(rangeStart) || !start.Before(rangeEnd) {
		return freeBusyPeriod{}, false
	}

	if start.Before(rangeStart) {
		start = rangeStart
	}

	if end.After(rangeEnd) {
		end = rangeEnd
	}

	if !end.After(start) {
		return freeBusyPeriod{}, false
	}

	return freeBusyPeriod{Start: start, End: end}, true
}

func mergeFreeBusyPeriods(periods []freeBusyPeriod) []freeBusyPeriod {
	sort.Slice(periods, func(i, j int) bool { return periods[i].Start.Before(periods[j].Start) })

	var merged []freeBusyPeriod

	for _, period := range periods {
		last := len(merged) - 1
		if last < 0 || period.Start.After(merged[last].End) {
			merged = append(merged, period)
			continue
		}

		if period.End.After(merged[last].End) {
			merged[last].End = period.End
		}
	}

	return merged
}

func writeFreeBusy(w io.Writer, start, end time.Time, periods map[string][]freeBusyPeriod) error {
	now := time.Now().UTC()

	cal := ics.NewCalendar()
	cal.SetVersion("2.0")
	cal.SetProductId(freeBusyProductID)

	busy := ics.NewBusy(fmt.Sprintf("freebusy-%d@teapot", now.UnixNano()))
	busy.SetDtStampTime(now)
	busy.SetStartAt(start)
	busy.SetEndAt(end)

	for _, fbType := range []string{freeBusyTypeBusy, freeBusyTypeTentative, freeBusyTypeUnavailable} {
		values := make([]string, 0, len(periods[fbType]))
		for _, period := range periods[fbType] {
			values = append(values, period.Start.UTC().Format(dateWithUTCTimeLayout)+"/"+
				period.End.UTC().Format(dateWithUTCTimeLayout))
		}

		if len(values) == 0 {
			continue
		}

		busy.AddProperty(
			ics.ComponentPropertyFreebusy,
			strings.Join(values, ","),
			&ics.KeyValues{Key: string(ics.ParameterFbtype), Value: []string{fbType}},
		)
	}

	cal.AddVBusy(busy)

	return cal.SerializeTo(w, ics.WithNewLineWindows)
}
