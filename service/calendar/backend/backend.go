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
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/teapotovh/teapot/lib/observability"
	"github.com/teapotovh/teapot/lib/webdav/caldav"
	daverr "github.com/teapotovh/teapot/lib/webdav/error"
	"github.com/teapotovh/teapot/service/calendar/store"
)

var (
	ErrUnexpectedNilCalendar = errors.New("unexpected nil calendar")
	ErrUnexpectedNilObject   = errors.New("unexpected nil object")
	ErrETagDidNotMatch       = errors.New("etag did not match")

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
	ctx, span := observability.TracerFromContext(ctx).Start(ctx, "CalendarHomeSetPath")
	defer observability.SpanEnd(span, err)

	up, err := b.CurrentUserPrincipal(ctx)
	if err != nil {
		return "", fmt.Errorf("could not get user principal: %w", err)
	}

	return up + "/calendars/", nil
}

func caldavCalendarToStoreCalendar(cal *caldav.Calendar) store.Calendar {
	return store.Calendar{
		Path: normalizePath(cal.Path),
		Metadata: store.CalendarMetadata{
			Name:                  cal.Name,
			Description:           cal.Description,
			SupportedComponentSet: cal.SupportedComponentSet,
			MaxResourceSize:       cal.MaxResourceSize,
		},
	}
}

func storeCalendarToCaldavCalendar(cal store.Calendar) caldav.Calendar {
	return caldav.Calendar{
		Path:                  string(cal.Path),
		Name:                  cal.Metadata.Name,
		Description:           cal.Metadata.Description,
		SupportedComponentSet: cal.Metadata.SupportedComponentSet,
	}
}

func normalizePath(path string) store.Path {
	return store.Path(strings.TrimRight(filepath.Clean(path), "/"))
}

func (b *Backend) CreateCalendar(ctx context.Context, calendar *caldav.Calendar) (err error) {
	ctx, span := observability.TracerFromContext(ctx).Start(ctx, "CreateCalendars")
	defer observability.SpanEnd(span, err)

	if calendar == nil {
		return ErrUnexpectedNilCalendar
	}

	if len(calendar.SupportedComponentSet) <= 0 {
		calendar.SupportedComponentSet = SupportedComponentSet
	}

	err = b.store.CreateCalendar(ctx, caldavCalendarToStoreCalendar(calendar))
	if err != nil {
		return fmt.Errorf("error while creating calendar at path %q in storage: %w", calendar.Path, err)
	}

	return nil
}

func (b *Backend) ListCalendars(ctx context.Context) (calendars []caldav.Calendar, err error) {
	ctx, span := observability.TracerFromContext(ctx).Start(ctx, "ListCalendars")
	defer observability.SpanEnd(span, err)

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
	ctx, span := observability.TracerFromContext(ctx).Start(ctx, "GetCalendar")
	defer observability.SpanEnd(span, err)

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
	ctx, span := observability.TracerFromContext(ctx).Start(ctx, "PutCalendarObject")
	defer observability.SpanEnd(span, err)

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
	ctx, span := observability.TracerFromContext(ctx).Start(ctx, "GetCalendarObject")
	defer observability.SpanEnd(span, err)

	obj, err := b.store.GetCalendarObject(ctx, normalizePath(path))
	if err != nil {
		return nil, fmt.Errorf("error while fetching calendar object at path %q from storage: %w", path, err)
	}

	span.SetAttributes(attribute.String("etag", obj.ETag))

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

func (b *Backend) ListCalendarObjects(
	ctx context.Context,
	path string,
	req *caldav.CalendarCompRequest,
) (objects []caldav.CalendarObject, err error) {
	ctx, span := observability.TracerFromContext(ctx).Start(ctx, "ListCalendarObjects")
	defer observability.SpanEnd(span, err)

	return b.listCalendarObjects(ctx, path, req)
}

func (b *Backend) QueryCalendarObjects(
	ctx context.Context,
	path string,
	query *caldav.CalendarQuery,
) (objects []caldav.CalendarObject, err error) {
	ctx, span := observability.TracerFromContext(ctx).Start(ctx, "QueryCalendarObjects")
	defer observability.SpanEnd(span, err)

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
	ctx, span := observability.TracerFromContext(ctx).Start(ctx, "DeleteCalendarObject")
	defer observability.SpanEnd(span, err)

	if err := b.store.DeleteCalendarObject(ctx, normalizePath(path)); err != nil {
		return fmt.Errorf("error while deleting calendar object at path %q in storage: %w", path, err)
	}

	return nil
}

// listCalendarObjects is extracted from the ListCalendarObjects impl so it
// can be reused for QueryCalendarObjects.
func (b *Backend) listCalendarObjects(
	ctx context.Context,
	path string,
	_ *caldav.CalendarCompRequest,
) (objects []caldav.CalendarObject, err error) {
	ctx, span := observability.TracerFromContext(ctx).Start(ctx, "listCalendarObjects")
	defer observability.SpanEnd(span, err)

	objs, err := b.store.ListCalendarObjects(ctx, normalizePath(path))
	if err != nil {
		return nil, fmt.Errorf("error while fetching calendar objects at path %q from storage: %w", path, err)
	}

	for _, obj := range objs {
		span.AddEvent("retrieved calendar object", trace.WithAttributes(
			attribute.String("path", obj.Path.String()),
			attribute.String("etag", obj.ETag),
		))

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
