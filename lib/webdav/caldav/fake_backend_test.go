package caldav_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	ics "github.com/arran4/golang-ical"

	"github.com/teapotovh/teapot/lib/webdav"
	"github.com/teapotovh/teapot/lib/webdav/caldav"
	daverr "github.com/teapotovh/teapot/lib/webdav/error"
)

type fakeBackend struct {
	mu sync.Mutex

	calendars map[string]caldav.Calendar
	objects   map[string]caldav.CalendarObject

	hasPut              bool
	putPreconditions    webdav.Preconditions
	hasDelete           bool
	deletePreconditions webdav.Preconditions
}

var _ caldav.Backend = (*fakeBackend)(nil)

func newFakeBackend() *fakeBackend {
	return &fakeBackend{
		calendars: map[string]caldav.Calendar{},
		objects:   map[string]caldav.CalendarObject{},
	}
}

func (b *fakeBackend) CurrentUserPrincipal(_ context.Context) (string, error) {
	return "/user/", nil
}

func (b *fakeBackend) CalendarHomeSetPath(_ context.Context) (string, error) {
	return "/user/calendars/", nil
}

func (b *fakeBackend) CreateCalendar(_ context.Context, calendar *caldav.Calendar) error {
	if calendar == nil {
		return daverr.HTTPErrorf(http.StatusBadRequest, "fake: nil calendar")
	}

	stored := *calendar
	stored.Path = path.Clean(stored.Path)

	b.mu.Lock()
	defer b.mu.Unlock()

	b.calendars[stored.Path] = stored

	return nil
}

func (b *fakeBackend) ListCalendars(_ context.Context) ([]caldav.Calendar, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	calendars := make([]caldav.Calendar, 0, len(b.calendars))
	for _, calendar := range b.calendars {
		calendars = append(calendars, calendar)
	}

	return calendars, nil
}

func (b *fakeBackend) GetCalendar(_ context.Context, calendarPath string) (*caldav.Calendar, error) {
	key := path.Clean(calendarPath)

	b.mu.Lock()
	defer b.mu.Unlock()

	calendar, ok := b.calendars[key]
	if !ok {
		return nil, daverr.HTTPErrorf(http.StatusNotFound, "fake: calendar %q not found", calendarPath)
	}

	return &calendar, nil
}

func (b *fakeBackend) GetCalendarObject(
	_ context.Context,
	objectPath string,
	_ *caldav.CalendarCompRequest,
	pre webdav.Preconditions,
) (*caldav.CalendarObject, webdav.Outcome, error) {
	key := path.Clean(objectPath)

	b.mu.Lock()
	defer b.mu.Unlock()

	object, ok := b.objects[key]
	if !ok {
		return nil, webdav.Proceed, daverr.HTTPErrorf(
			http.StatusNotFound,
			"fake: calendar object %q not found",
			objectPath,
		)
	}

	outcome, err := pre.Evaluate(true, object.ETag)
	if err != nil {
		return nil, webdav.Proceed, err
	}

	return &object, outcome, nil
}

func (b *fakeBackend) ListCalendarObjects(
	_ context.Context,
	calendarPath string,
	_ *caldav.CalendarCompRequest,
) ([]caldav.CalendarObject, error) {
	prefix := path.Clean(calendarPath) + "/"

	b.mu.Lock()
	defer b.mu.Unlock()

	var objects []caldav.CalendarObject
	for objectPath, object := range b.objects {
		if strings.HasPrefix(objectPath, prefix) {
			objects = append(objects, object)
		}
	}

	return objects, nil
}

func (b *fakeBackend) QueryCalendarObjects(
	ctx context.Context,
	calendarPath string,
	query *caldav.CalendarQuery,
) ([]caldav.CalendarObject, error) {
	objects, err := b.ListCalendarObjects(ctx, calendarPath, nil)
	if err != nil {
		return nil, err
	}

	if query == nil {
		return objects, nil
	}

	return caldav.Filter(&query.CompFilter, objects)
}

func (b *fakeBackend) PutCalendarObject(
	_ context.Context,
	objectPath string,
	calendar *ics.Calendar,
	pre webdav.Preconditions,
) (*caldav.CalendarObject, bool, error) {
	if calendar == nil {
		return nil, false, daverr.HTTPErrorf(http.StatusBadRequest, "fake: nil calendar object")
	}

	var buf bytes.Buffer
	if err := calendar.SerializeTo(&buf); err != nil {
		return nil, false, fmt.Errorf("fake: could not serialize calendar object: %w", err)
	}

	data := buf.Bytes()
	key := path.Clean(objectPath)
	object := caldav.CalendarObject{
		Path:          key,
		ModTime:       time.Now(),
		ContentLength: int64(len(data)),
		ETag:          fmt.Sprintf("%x", sha256.Sum256(data)),
		Data:          calendar,
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	_, exists := b.objects[key]
	b.objects[key] = object
	b.hasPut = true
	b.putPreconditions = pre

	return &object, !exists, nil
}

func (b *fakeBackend) DeleteCalendarObject(
	_ context.Context,
	objectPath string,
	pre webdav.Preconditions,
) error {
	key := path.Clean(objectPath)

	b.mu.Lock()
	defer b.mu.Unlock()

	if _, ok := b.objects[key]; !ok {
		return daverr.HTTPErrorf(http.StatusNotFound, "fake: calendar object %q not found", objectPath)
	}

	b.deletePreconditions = pre
	b.hasDelete = true

	delete(b.objects, key)

	return nil
}

func (b *fakeBackend) lastPutPreconditions() (webdav.Preconditions, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.putPreconditions, b.hasPut
}

func (b *fakeBackend) lastDeletePreconditions() (webdav.Preconditions, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.deletePreconditions, b.hasDelete
}
