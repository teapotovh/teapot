package backend

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/emersion/go-ical"

	"github.com/teapotovh/teapot/lib/webdav/caldav"
	daverr "github.com/teapotovh/teapot/lib/webdav/error"
	"github.com/teapotovh/teapot/service/calendar/store"
)

var (
	ErrUnexpectedNilCalendar = errors.New("unexpected nil calendar")
	ErrUnexpectedNilObject   = errors.New("unexpected nil object")
	ErrETagDidNotMatch       = errors.New("etag did not match")

	MaxResourceSize       = int64(0)
	SupportedComponentSet = []string{"VEVENT", "VTODO", "VJOURNAL", "VFREEBUSY"}
)

type Backend struct {
	userPrincipal

	logger *slog.Logger

	store store.Store
}

func NewBackend(store store.Store, logger *slog.Logger) *Backend {
	return &Backend{
		logger: logger,

		store: store,
	}
}

func (b *Backend) CalendarHomeSetPath(ctx context.Context) (path string, err error) {
	defer func() { b.logCall(ctx, "CalendarHomeSetPath", "", err, time.Now()) }()

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

func (b *Backend) CreateCalendar(ctx context.Context, calendar *caldav.Calendar) (err error) {
	defer func() { b.logCall(ctx, "CreateCalendar", calendar.Path, err, time.Now()) }()

	if calendar == nil {
		return ErrUnexpectedNilCalendar
	}

	err = b.store.CreateCalendar(ctx, caldavCalendarToStoreCalendar(calendar))
	if err != nil {
		return fmt.Errorf("error while creating calendar at path %q in storage: %w", calendar.Path, err)
	}

	return nil
}

func (b *Backend) ListCalendars(ctx context.Context) (calendars []caldav.Calendar, err error) {
	defer func() { b.logCall(ctx, "ListCalendars", "", err, time.Now(), "calendars", calendars) }()

	path, err := b.CalendarHomeSetPath(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not get home-set path: %w", err)
	}

	cals, err := b.store.ListCalendars(ctx, normalizePath(path))
	if err != nil {
		return nil, fmt.Errorf("error while fetching calendars at path %q from storage: %w", path, err)
	}

	for _, cal := range cals {
		calendars = append(calendars, storeCalendarToCaldavCalendar(cal))
	}

	return calendars, nil
}

func (b *Backend) GetCalendar(ctx context.Context, path string) (calendar *caldav.Calendar, err error) {
	defer func() { b.logCall(ctx, "GetCalendar", path, err, time.Now(), "calendar", calendar) }()

	cal, err := b.store.GetCalendar(ctx, normalizePath(path))
	if err != nil {
		return nil, fmt.Errorf("error while getting calendar at path %q in storage: %w", path, err)
	}

	c := storeCalendarToCaldavCalendar(*cal)

	return &c, nil
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
	cal, err := obj.Calendar()
	if err != nil {
		return nil, fmt.Errorf("error while parsing ical object and generating etag: %w", err)
	}

	calendarObject := caldav.CalendarObject{
		Path:          string(obj.Path),
		ModTime:       obj.ModTime,
		ContentLength: obj.Size(),
		ETag:          obj.ETag,
		Data:          cal,
	}

	return &calendarObject, nil
}

func (b *Backend) PutCalendarObject(
	ctx context.Context,
	path string,
	calendar *ical.Calendar,
	opts *caldav.PutCalendarObjectOptions,
) (object *caldav.CalendarObject, err error) {
	defer func() { b.logCall(ctx, "PutCalendarObject", path, err, time.Now(), "opts", opts) }()

	if calendar == nil {
		return nil, ErrUnexpectedNilObject
	}

	if opts != nil {
		matchers := make([]ETagMatcher, 0, 2)
		if opts.IfMatch.IsSet() {
			matchers = append(matchers, ETagMatcher(opts.IfMatch.MatchETag))
		}

		if opts.IfNoneMatch.IsSet() {
			matchers = append(matchers, NegateETagMatch(ETagMatcher(opts.IfNoneMatch.MatchETag)))
		}

		matcher := AndETagMatch(matchers...)

		obj, err := b.store.GetCalendarObject(ctx, normalizePath(path))
		if err != nil {
			if !errors.Is(err, store.ErrNotFound) {
				return nil, fmt.Errorf("error while checking for previous object at path %q: %w", path, err)
			}
			// We ignore NotFound errors, insertion is safe on the first insertion of an object
		} else {
			etag := obj.ETag

			match, err := matcher(etag)
			if err != nil {
				return nil, &daverr.HTTPError{
					Code: http.StatusPreconditionFailed,
					Err:  fmt.Errorf("error while matching etag: %w", err),
				}
			}

			if !match {
				return nil, ErrETagDidNotMatch
			}
		}
	}

	objPtr, err := caldavObjectToStoreObject(path, calendar)
	if err != nil {
		return nil, fmt.Errorf("could not convert caldav Object at path %q into store Object: %w", path, err)
	}

	obj := *objPtr
	if err := b.store.CreateCalendarObject(ctx, obj); err != nil {
		return nil, fmt.Errorf("error while creating calendar object at path %q in storage: %w", path, err)
	}

	object, err = storeObjectToCaldavObject(obj)
	if err != nil {
		return nil, fmt.Errorf("error while converting stored object back to a caldav CalendarObject: %w", err)
	}

	return object, nil
}

func (b *Backend) GetCalendarObject(
	ctx context.Context,
	path string,
	req *caldav.CalendarCompRequest,
) (object *caldav.CalendarObject, err error) {
	defer func() {
		var extra []any
		if object != nil {
			extra = append(extra, "etag", object.ETag)
		}

		b.logCall(ctx, "GetCalendarObject", path, err, time.Now(), extra...)
	}()

	obj, err := b.store.GetCalendarObject(ctx, normalizePath(path))
	if err != nil {
		return nil, fmt.Errorf("error while fetching calendar object at path %q from storage: %w", path, err)
	}

	object, err = storeObjectToCaldavObject(*obj)
	if err != nil {
		return nil, fmt.Errorf("error while converting object at path %q to a caldav CalendarObject: %w", obj.Path, err)
	}

	// object, err = mapCalendarObject(object, req)
	// if err != nil {
	// 	return nil, fmt.Errorf("error while applying filters and maps to calendar object: %w", err)
	// }

	return object, nil
}

func pathsAndEtags(objects []caldav.CalendarObject) (paths []string, etags []string) {
	for _, object := range objects {
		paths = append(paths, object.Path)
		etags = append(etags, object.ETag)
	}

	return paths, etags
}

func (b *Backend) ListCalendarObjects(
	ctx context.Context,
	path string,
	req *caldav.CalendarCompRequest,
) (objects []caldav.CalendarObject, err error) {
	defer func() {
		paths, etags := pathsAndEtags(objects)
		b.logCall(ctx, "ListCalendarObjects", path, err, time.Now(), "paths", paths, "etags", etags)
	}()

	return b.listCalendarObjects(ctx, path, req)
}

func (b *Backend) QueryCalendarObjects(
	ctx context.Context,
	path string,
	query *caldav.CalendarQuery,
) (objects []caldav.CalendarObject, err error) {
	defer func() {
		paths, etags := pathsAndEtags(objects)
		b.logCall(ctx, "QueryCalendarObjects", path, err, time.Now(), "paths", paths, "etags", etags)
	}()

	objects, err = b.listCalendarObjects(ctx, path, &query.CompRequest)
	if err != nil {
		return nil, fmt.Errorf("error while listing all calendar objects for query: %w", err)
	}

	objects, err = caldav.Filter(&query.CompFilter, objects)
	if err != nil {
		return nil, fmt.Errorf("error while filtering down calendar objects: %w", err)
	}

	return objects, nil
}

func (b *Backend) DeleteCalendarObject(ctx context.Context, path string) (err error) {
	defer func() { b.logCall(ctx, "DeleteCalendarObject", path, err, time.Now()) }()

	if err := b.store.DeleteCalendarObject(ctx, normalizePath(path)); err != nil {
		return fmt.Errorf("error while deleting calendar object at path %q in storage: %w", path, err)
	}

	return nil
}

func (b *Backend) logCall(ctx context.Context, method string, path string, err error, start time.Time, extra ...any) {
	fields := []any{"method", method, "duration", time.Since(start)}
	if len(path) > 0 {
		fields = append(fields, "path", path)
	}

	fields = append(fields, extra...)
	if err != nil {
		fields = append(fields, "err", err)
		b.logger.ErrorContext(ctx, "called", fields...)
	} else {
		b.logger.InfoContext(ctx, "called", fields...)
	}
}

// listCalendarObjects is extracted from the ListCalendarObjects impl so it
// can be reused for QueryCalendarObjects.
func (b *Backend) listCalendarObjects(
	ctx context.Context,
	path string,
	req *caldav.CalendarCompRequest,
) (objects []caldav.CalendarObject, err error) {
	objs, err := b.store.ListCalendarObjects(ctx, normalizePath(path))
	if err != nil {
		return nil, fmt.Errorf("error while fetching calendar objects at path %q from storage: %w", path, err)
	}

	for _, obj := range objs {
		object, err := storeObjectToCaldavObject(obj)
		if err != nil {
			return nil, fmt.Errorf(
				"error while converting object at path %q to a caldav CalendarObject: %w",
				obj.Path,
				err,
			)
		}

		// object, err = mapCalendarObject(object, req)
		// if err != nil {
		// 	return nil, fmt.Errorf("error while applying filters and maps to calendar object %q: %w", obj.Path, err)
		// }

		objects = append(objects, *object)
	}

	return objects, nil
}

// Ensure Backend implements caldav.Backend.
var _ caldav.Backend = &Backend{}
