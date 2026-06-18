package backend

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/emersion/go-ical"

	"github.com/teapotovh/teapot/lib/webdav/caldav"
	"github.com/teapotovh/teapot/service/calendar/store"
)

var (
	errUnimplemented         = errors.New("operation not implemented")
	ErrUnexpectedNilCalendar = errors.New("unexpected nil calendar")
	ErrUnexpectedNilObject   = errors.New("unexpected nil object")

	MaxResourceSize       = int64(0)
	SupportedComponentSet = []string{"VEVENT", "VTODO", "VJOURNAL", "VFREEBUSY"}
)

type Backend struct {
	logger *slog.Logger

	store store.Store

	userPrincipal
}

func NewBackend(store store.Store, logger *slog.Logger) *Backend {
	return &Backend{
		logger: logger,

		store: store,
	}
}

func (b *Backend) CalendarHomeSetPath(ctx context.Context) (string, error) {
	b.logger.InfoContext(ctx, "called", "method", "CalendarHomeSetPath")

	up, err := b.CurrentUserPrincipal(ctx)
	if err != nil {
		return "", fmt.Errorf("could not get user principal: %w", err)
	}

	return up + "/calendars/", nil
}

func caldavCalendarToStoreCalendar(cal *caldav.Calendar) store.Calendar {
	return store.Calendar{
		Path:        normalizePath(cal.Path),
		Name:        cal.Name,
		Description: cal.Description,
	}
}

func storeCalendarToCaldavCalendar(cal store.Calendar) caldav.Calendar {
	return caldav.Calendar{
		Path:        string(cal.Path),
		Name:        cal.Name,
		Description: cal.Description,

		MaxResourceSize:       MaxResourceSize,
		SupportedComponentSet: SupportedComponentSet,
	}
}

func normalizePath(path string) store.Path {
	return store.Path(strings.TrimRight(filepath.Clean(path), "/"))
}

func (b *Backend) CreateCalendar(ctx context.Context, calendar *caldav.Calendar) error {
	b.logger.InfoContext(ctx, "called", "method", "CreateCalendar", "calendar", calendar)

	if calendar == nil {
		return ErrUnexpectedNilCalendar
	}

	err := b.store.CreateCalendar(ctx, caldavCalendarToStoreCalendar(calendar))
	if err != nil {
		return fmt.Errorf("error while creating calendar under path %q in storage: %w", calendar.Path, err)
	}

	return nil
}

func (b *Backend) ListCalendars(ctx context.Context) ([]caldav.Calendar, error) {
	b.logger.InfoContext(ctx, "called", "method", "ListCalendars")

	path, err := b.CalendarHomeSetPath(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not get home-set path: %w", err)
	}

	cals, err := b.store.ListCalendars(ctx, normalizePath(path))
	if err != nil {
		return nil, fmt.Errorf("error while fetching calendars under path %q from storage: %w", path, err)
	}

	var calendars []caldav.Calendar
	for _, cal := range cals {
		calendars = append(calendars, storeCalendarToCaldavCalendar(cal))
	}

	return calendars, nil
}

func (b *Backend) GetCalendar(ctx context.Context, path string) (*caldav.Calendar, error) {
	b.logger.InfoContext(ctx, "called", "method", "GetCalendar", "path", path)

	cal, err := b.store.GetCalendar(ctx, normalizePath(path))
	if err != nil {
		return nil, fmt.Errorf("error while getting calendar under path %q in storage: %w", path, err)
	}

	calendar := storeCalendarToCaldavCalendar(*cal)

	return &calendar, nil
}

func caldavObjectToStoreObject(
	path string,
	calendar *ical.Calendar,
) (*store.Object, error) {
	return store.SerializeObject(caldav.CalendarObject{
		Path:    path,
		ModTime: time.Now(),
		Data:    calendar,
	})
}

func storeObjectToCaldavObject(obj store.Object) (*caldav.CalendarObject, error) {
	cal, etag, err := obj.CalAndETag()
	if err != nil {
		return nil, fmt.Errorf("error while parsing ical object and generating etag: %w", err)
	}

	calendarObject := caldav.CalendarObject{
		Path:          string(obj.Path),
		ModTime:       obj.ModTime,
		ContentLength: obj.Size(),
		ETag:          etag,
		Data:          cal,
	}
	return &calendarObject, nil
}

func (b *Backend) PutCalendarObject(
	ctx context.Context,
	path string,
	calendar *ical.Calendar,
	opts *caldav.PutCalendarObjectOptions,
) (*caldav.CalendarObject, error) {
	b.logger.InfoContext(ctx, "called", "method", "PutCalendarObject", "path", path, "calendar", calendar, "opts", opts)

	if calendar == nil {
		return nil, ErrUnexpectedNilObject
	}

	// TODO: handle opts

	objPtr, err := caldavObjectToStoreObject(path, calendar)
	if err != nil {
		return nil, fmt.Errorf("could not convert caldav Object into store Object for path %q: %w", path, err)
	}

	obj := *objPtr
	if err := b.store.CreateCalendarObject(ctx, obj); err != nil {
		return nil, fmt.Errorf("error while creating calendar object under path %q in storage: %w", path, err)
	}

	object, err := storeObjectToCaldavObject(obj)
	if err != nil {
		return nil, fmt.Errorf("error while converting stored object back to a caldav CalendarObject: %w", err)
	}

	return object, nil
}

func (b *Backend) GetCalendarObject(
	ctx context.Context,
	path string,
	req *caldav.CalendarCompRequest,
) (*caldav.CalendarObject, error) {
	b.logger.InfoContext(ctx, "called", "method", "GetCalendarObject", "path", path, "req", req)

	// TODO: handle opts

	obj, err := b.store.GetCalendarObject(ctx, normalizePath(path))
	if err != nil {
		return nil, fmt.Errorf("error while fetching calendar object at path %q from storage: %w", path, err)
	}

	object, err := storeObjectToCaldavObject(*obj)
	if err != nil {
		return nil, fmt.Errorf("error while converting object at path %q to a caldav CalendarObject: %w", obj.Path, err)
	}

	return object, nil
}

func (b *Backend) ListCalendarObjects(
	ctx context.Context,
	path string,
	req *caldav.CalendarCompRequest,
) ([]caldav.CalendarObject, error) {
	b.logger.InfoContext(ctx, "called", "method", "ListCalendarObjects", "path", path, "req", req)

	objs, err := b.store.ListCalendarObjects(ctx, normalizePath(path))
	if err != nil {
		return nil, fmt.Errorf("error while fetching calendar objects under path %q from storage: %w", path, err)
	}

	var objects []caldav.CalendarObject
	for _, obj := range objs {
		object, err := storeObjectToCaldavObject(obj)
		if err != nil {
			return nil, fmt.Errorf("error while converting object at path %q to a caldav CalendarObject: %w", obj.Path, err)
		}

		objects = append(objects, *object)
	}

	return objects, nil
}

func (b *Backend) QueryCalendarObjects(
	ctx context.Context,
	path string,
	query *caldav.CalendarQuery,
) ([]caldav.CalendarObject, error) {
	// caldav.Filter()

	return nil, fmt.Errorf("query calendar objects: %w", errUnimplemented)
}

func (b *Backend) DeleteCalendarObject(ctx context.Context, path string) error {
	return fmt.Errorf("delete calendar object: %w", errUnimplemented)
}

// Ensure Backend implements caldav.Backend.
var _ caldav.Backend = &Backend{}
