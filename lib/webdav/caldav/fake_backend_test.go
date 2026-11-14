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

	"github.com/teapotovh/teapot/lib/webdav/caldav"
	daverr "github.com/teapotovh/teapot/lib/webdav/error"
)

type fakeBackend struct {
	mu sync.Mutex

	calendars map[string]caldav.Calendar
	objects   map[string]caldav.CalendarObject

	hasPut     bool
	putOptions caldav.PutCalendarObjectOptions
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
) (*caldav.CalendarObject, error) {
	key := path.Clean(objectPath)

	b.mu.Lock()
	defer b.mu.Unlock()

	object, ok := b.objects[key]
	if !ok {
		return nil, daverr.HTTPErrorf(http.StatusNotFound, "fake: calendar object %q not found", objectPath)
	}

	return &object, nil
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
	opts *caldav.PutCalendarObjectOptions,
) (*caldav.CalendarObject, error) {
	if calendar == nil {
		return nil, daverr.HTTPErrorf(http.StatusBadRequest, "fake: nil calendar object")
	}

	var buf bytes.Buffer
	if err := calendar.SerializeTo(&buf); err != nil {
		return nil, fmt.Errorf("fake: could not serialize calendar object: %w", err)
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

	b.objects[key] = object
	b.hasPut = true

	if opts != nil {
		b.putOptions = *opts
	}

	return &object, nil
}

func (b *fakeBackend) DeleteCalendarObject(_ context.Context, objectPath string) error {
	key := path.Clean(objectPath)

	b.mu.Lock()
	defer b.mu.Unlock()

	if _, ok := b.objects[key]; !ok {
		return daverr.HTTPErrorf(http.StatusNotFound, "fake: calendar object %q not found", objectPath)
	}

	delete(b.objects, key)

	return nil
}

func (b *fakeBackend) lastPutOptions() (caldav.PutCalendarObjectOptions, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.putOptions, b.hasPut
}
