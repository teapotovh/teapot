package backend

import (
	"github.com/teapotovh/teapot/lib/webdav/caldav"
	"github.com/teapotovh/teapot/lib/wideevent"
	"github.com/teapotovh/teapot/service/calendar/store"
)

type CalendarHomeSetPathWideEvent struct {
	wideevent.AlwaysEmit

	UserPrincipal string
}

func (c *CalendarHomeSetPathWideEvent) Fields() wideevent.Fields { return wideevent.ExtractFields(c) }

type CalendarWideEvent struct {
	Path        string
	Name        string
	Description string
}

func wideEventCalendarFromCaldav(cal *caldav.Calendar) CalendarWideEvent {
	return CalendarWideEvent{
		Path:        cal.Path,
		Name:        cal.Name,
		Description: cal.Description,
	}
}

type CreateCalendarWideEvent struct {
	wideevent.AlwaysEmit

	Calendar CalendarWideEvent
}

func (c *CreateCalendarWideEvent) Fields() wideevent.Fields { return wideevent.ExtractFields(c) }

func (c *CreateCalendarWideEvent) withCalendar(cal *caldav.Calendar) {
	c.Calendar = wideEventCalendarFromCaldav(cal)
}

type ListCalendarsWideEvent struct {
	wideevent.AlwaysEmit

	Path      store.Path
	Calendars []CalendarWideEvent
}

func (l *ListCalendarsWideEvent) Fields() wideevent.Fields { return wideevent.ExtractFields(l) }

func (l *ListCalendarsWideEvent) withCalendars(cals []caldav.Calendar) {
	names := make([]CalendarWideEvent, 0, len(cals))
	for _, cal := range cals {
		names = append(names, wideEventCalendarFromCaldav(&cal))
	}

	l.Calendars = names
}

type GetCalendarWideEvent struct {
	wideevent.AlwaysEmit

	Path     store.Path
	Calendar CalendarWideEvent
}

func (g *GetCalendarWideEvent) Fields() wideevent.Fields { return wideevent.ExtractFields(g) }

func (g *GetCalendarWideEvent) withCalendar(cal *caldav.Calendar) {
	g.Calendar = wideEventCalendarFromCaldav(cal)
}

type ToCaldavObjectWideEvent struct {
	wideevent.NeverEmit

	Path store.Path
	ETag string
	Size int
}

func (t *ToCaldavObjectWideEvent) Fields() wideevent.Fields { return wideevent.ExtractFields(t) }

func (t *ToCaldavObjectWideEvent) withObject(obj *store.Object) {
	t.Path = obj.Path
	t.ETag = obj.ETag
	t.Size = len(obj.Data)
}

type CalendarObjectWideEvent struct {
	Path       string
	ETag       string
	Components int
}

func wideEventCalendarObjectFromCaldav(obj *caldav.CalendarObject) CalendarObjectWideEvent {
	return CalendarObjectWideEvent{
		Path:       obj.Path,
		ETag:       obj.ETag,
		Components: len(obj.Data.Components),
	}
}

type PutCalendarObjectWideEvent struct {
	wideevent.AlwaysEmit

	Path           string
	ETag           string
	MatchETag      string
	NoneMatchETag  string
	CalendarObject CalendarObjectWideEvent
}

func (p *PutCalendarObjectWideEvent) Fields() wideevent.Fields { return wideevent.ExtractFields(p) }

func (p *PutCalendarObjectWideEvent) withCalendarObject(obj *caldav.CalendarObject) {
	p.CalendarObject = wideEventCalendarObjectFromCaldav(obj)
}

type GetCalendarObjectWideEvent struct {
	wideevent.AlwaysEmit

	Path           store.Path
	CalendarObject CalendarObjectWideEvent
}

func (g *GetCalendarObjectWideEvent) Fields() wideevent.Fields { return wideevent.ExtractFields(g) }

func (g *GetCalendarObjectWideEvent) withCalendarObject(obj *caldav.CalendarObject) {
	g.CalendarObject = wideEventCalendarObjectFromCaldav(obj)
}

type ListCalendarObjectWideEvent struct {
	wideevent.AlwaysEmit

	Path    store.Path
	Objects int
}

func (l *ListCalendarObjectWideEvent) Fields() wideevent.Fields { return wideevent.ExtractFields(l) }

type QueryCalendarObjectWideEvent struct {
	wideevent.AlwaysEmit

	All      int
	Filtered int
}

func (q *QueryCalendarObjectWideEvent) Fields() wideevent.Fields { return wideevent.ExtractFields(q) }

type DeleteCalendarObjectWideEvent struct {
	wideevent.AlwaysEmit

	Path store.Path
}

func (d *DeleteCalendarObjectWideEvent) Fields() wideevent.Fields { return wideevent.ExtractFields(d) }
