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

	ics "github.com/arran4/golang-ical"
	"golang.org/x/sync/errgroup"

	"github.com/teapotovh/teapot/lib/webdav/caldav"
	daverr "github.com/teapotovh/teapot/lib/webdav/error"
	"github.com/teapotovh/teapot/lib/wideevent"
	"github.com/teapotovh/teapot/service/calendar/store"
)

const MaxDecodesInParallel = 32

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
	ctx, we, handle := wideevent.Start[CalendarHomeSetPathWideEvent](ctx, "CalendarHomeSetPath")
	defer func() { handle.End(err) }()

	up, err := b.CurrentUserPrincipal(ctx)
	if err != nil {
		return "", fmt.Errorf("could not get user principal: %w", err)
	}

	we.UserPrincipal = up

	return up + "/calendars/", nil
}

func caldavCalendarToStoreCalendar(cal *caldav.Calendar) store.Calendar {
	return store.Calendar{
		Path: normalizePath(cal.Path),
		Metadata: store.CalendarMetadata{
			Name:                  cal.Name,
			Description:           cal.Description,
			Color:                 cal.Color,
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
		Color:                 cal.Metadata.Color,
		SupportedComponentSet: cal.Metadata.SupportedComponentSet,
	}
}

func normalizePath(path string) store.Path {
	return store.Path(strings.TrimRight(filepath.Clean(path), "/"))
}

func (b *Backend) CreateCalendar(ctx context.Context, calendar *caldav.Calendar) (err error) {
	ctx, we, handle := wideevent.Start[CreateCalendarWideEvent](ctx, "CreateCalendars")
	defer func() { handle.End(err) }()

	if calendar == nil {
		return ErrUnexpectedNilCalendar
	}

	if len(calendar.SupportedComponentSet) <= 0 {
		calendar.SupportedComponentSet = SupportedComponentSet
	}

	we.withCalendar(calendar)

	err = b.store.CreateCalendar(ctx, caldavCalendarToStoreCalendar(calendar))
	if err != nil {
		return fmt.Errorf("error while creating calendar at path %q in storage: %w", calendar.Path, err)
	}

	return nil
}

func (b *Backend) ListCalendars(ctx context.Context) (calendars []caldav.Calendar, err error) {
	ctx, we, handle := wideevent.Start[ListCalendarsWideEvent](ctx, "ListCalendars")
	defer func() { handle.End(err) }()

	path, err := b.CalendarHomeSetPath(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not get home-set path: %w", err)
	}

	storePath := normalizePath(path)
	we.Path = storePath

	cals, err := b.store.ListCalendars(ctx, storePath)
	if err != nil {
		return nil, fmt.Errorf("error while fetching calendars at path %q from storage: %w", path, err)
	}

	for _, cal := range cals {
		calendars = append(calendars, storeCalendarToCaldavCalendar(cal))
	}

	we.withCalendars(calendars)

	return calendars, nil
}

func (b *Backend) GetCalendar(ctx context.Context, path string) (calendar *caldav.Calendar, err error) {
	ctx, we, handle := wideevent.Start[GetCalendarWideEvent](ctx, "GetCalendar")
	defer func() { handle.End(err) }()

	storePath := normalizePath(path)
	we.Path = storePath

	cal, err := b.store.GetCalendar(ctx, storePath)
	if err != nil {
		return nil, fmt.Errorf("error while getting calendar at path %q in storage: %w", path, err)
	}

	c := storeCalendarToCaldavCalendar(*cal)

	we.withCalendar(&c)

	return &c, nil
}

func caldavObjectToStoreObject(
	path string,
	calendar *ics.Calendar,
) (*store.Object, error) {
	return store.SerializeObject(caldav.CalendarObject{
		Path:    path,
		ModTime: time.Now(),
		Data:    calendar,
	})
}

func storeObjectToCaldavObject(ctx context.Context, obj store.Object) (co *caldav.CalendarObject, err error) {
	_, we, handle := wideevent.Start[ToCaldavObjectWideEvent](ctx, "storeObjectToCaldavObject")
	defer func() { handle.End(err) }()

	we.withObject(&obj)

	cal, err := obj.Calendar()
	if err != nil {
		return nil, fmt.Errorf("error while parsing ics object and generating etag: %w", err)
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
	calendar *ics.Calendar,
	opts *caldav.PutCalendarObjectOptions,
) (object *caldav.CalendarObject, err error) {
	ctx, we, handle := wideevent.Start[PutCalendarObjectWideEvent](ctx, "PutCalendarObject")
	defer func() { handle.End(err) }()

	if calendar == nil {
		return nil, ErrUnexpectedNilObject
	}

	we.Path = path

	if opts != nil {
		matchers := make([]ETagMatcher, 0, 2)

		if opts.IfMatch.IsSet() {
			we.MatchETag = string(opts.IfMatch)
			matchers = append(matchers, ETagMatcher(opts.IfMatch.MatchETag))
		}

		if opts.IfNoneMatch.IsSet() {
			we.NoneMatchETag = string(opts.IfNoneMatch)
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
			we.ETag = etag

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

	object, err = storeObjectToCaldavObject(ctx, obj)
	if err != nil {
		return nil, fmt.Errorf("error while converting stored object back to a caldav CalendarObject: %w", err)
	}

	we.withCalendarObject(object)

	return object, nil
}

func (b *Backend) GetCalendarObject(
	ctx context.Context,
	path string,
	req *caldav.CalendarCompRequest,
) (object *caldav.CalendarObject, err error) {
	ctx, we, handle := wideevent.Start[GetCalendarObjectWideEvent](ctx, "GetCalendarObject")
	defer func() { handle.End(err) }()

	storePath := normalizePath(path)
	we.Path = storePath

	obj, err := b.store.GetCalendarObject(ctx, storePath)
	if err != nil {
		return nil, fmt.Errorf("error while fetching calendar object at path %q from storage: %w", path, err)
	}

	object, err = storeObjectToCaldavObject(ctx, *obj)
	if err != nil {
		return nil, fmt.Errorf("error while converting object at path %q to a caldav CalendarObject: %w", obj.Path, err)
	}

	we.withCalendarObject(object)

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
	ctx, we, handle := wideevent.Start[ListCalendarObjectWideEvent](ctx, "ListCalendarObjects")
	defer func() { handle.End(err) }()

	storePath := normalizePath(path)
	we.Path = storePath

	objs, err := b.store.ListCalendarObjects(ctx, storePath)
	if err != nil {
		return nil, fmt.Errorf("error while fetching calendar objects at path %q from storage: %w", path, err)
	}

	objects = make([]caldav.CalendarObject, len(objs))
	eg, ctx := errgroup.WithContext(ctx)
	eg.SetLimit(MaxDecodesInParallel)

	for i, obj := range objs {
		eg.Go(func() error {
			object, err := storeObjectToCaldavObject(ctx, obj)
			if err != nil {
				return fmt.Errorf(
					"error while converting object #%d at path %q to a caldav CalendarObject: %w",
					i,
					obj.Path,
					err,
				)
			}

			// object, err = mapCalendarObject(object, req)
			// if err != nil {
			// 	return nil, fmt.Errorf("error while applying filters and maps to calendar object %q: %w", obj.Path, err)
			// }

			objects[i] = *object

			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}

	we.Objects = len(objects)

	return objects, nil
}

func (b *Backend) QueryCalendarObjects(
	ctx context.Context,
	path string,
	query *caldav.CalendarQuery,
) (objects []caldav.CalendarObject, err error) {
	ctx, we, handle := wideevent.Start[QueryCalendarObjectWideEvent](ctx, "QueryCalendarObjects")
	defer func() { handle.End(err) }()

	objects, err = b.ListCalendarObjects(ctx, path, &query.CompRequest)
	if err != nil {
		return nil, fmt.Errorf("error while listing all calendar objects for query: %w", err)
	}

	we.All = len(objects)

	objects, err = caldav.Filter(&query.CompFilter, objects)
	if err != nil {
		return nil, fmt.Errorf("error while filtering down calendar objects: %w", err)
	}

	we.Filtered = len(objects)

	return objects, nil
}

func (b *Backend) DeleteCalendarObject(ctx context.Context, path string) (err error) {
	ctx, we, handle := wideevent.Start[DeleteCalendarObjectWideEvent](ctx, "DeleteCalendarObject")
	defer func() { handle.End(err) }()

	storePath := normalizePath(path)
	we.Path = storePath

	if err := b.store.DeleteCalendarObject(ctx, storePath); err != nil {
		return fmt.Errorf("error while deleting calendar object at path %q in storage: %w", path, err)
	}

	return nil
}

// Ensure Backend implements caldav.Backend.
var _ caldav.Backend = &Backend{}
