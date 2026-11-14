package backend

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"

	ics "github.com/arran4/golang-ical"
	"golang.org/x/sync/errgroup"

	"github.com/teapotovh/teapot/lib/webdav"
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

func normalizePath(p string) store.Path {
	return store.Path(strings.TrimRight(path.Clean(p), "/"))
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
		err = fmt.Errorf("error while creating calendar at path %q in storage: %w", calendar.Path, err)
		if errors.Is(err, store.ErrCalendarAlreadyExists) {
			return &daverr.HTTPError{Code: http.StatusForbidden, Err: err}
		}

		return err
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
		err = fmt.Errorf("error while getting calendar at path %q in storage: %w", path, err)
		if errors.Is(err, store.ErrNotFound) {
			return nil, &daverr.HTTPError{Code: http.StatusNotFound, Err: err}
		}

		return nil, err
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
	objectPath string,
	calendar *ics.Calendar,
	opts *caldav.PutCalendarObjectOptions,
) (object *caldav.CalendarObject, created bool, err error) {
	ctx, we, handle := wideevent.Start[PutCalendarObjectWideEvent](ctx, "PutCalendarObject")
	defer func() { handle.End(err) }()

	if calendar == nil {
		return nil, false, ErrUnexpectedNilObject
	}

	storePath := normalizePath(objectPath)
	we.Path = objectPath

	existing, err := b.store.GetCalendarObject(ctx, storePath)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, false, fmt.Errorf("error while checking for previous object at path %q: %w", objectPath, err)
	}

	exists := err == nil
	if exists {
		we.ETag = existing.ETag
	}

	if err := checkPutPreconditions(existing, exists, opts, we); err != nil {
		return nil, false, err
	}

	if exists {
		existingCalendar, err := existing.Calendar()
		if err != nil {
			return nil, false, fmt.Errorf("error while parsing existing calendar object at path %q: %w", storePath, err)
		}

		if calendarUID(calendar) != calendarUID(existingCalendar) {
			return nil, false, fmt.Errorf(
				"calendar object at path %q changes its uid: %w",
				storePath,
				caldav.NewPreconditionError(caldav.PreconditionNoUIDConflict),
			)
		}
	}

	// A calendar object is a direct child of a calendar collection (RFC 4791 section 4.2).
	calendarPath := path.Dir(string(storePath))
	if cal, err := b.GetCalendar(ctx, calendarPath); err != nil {
		if !daverr.IsNotFound(err) {
			return nil, false, fmt.Errorf("error while getting calendar at path %q: %w", calendarPath, err)
		}
	} else if err := checkSupportedComponents(cal.SupportedComponentSet, calendar); err != nil {
		return nil, false, fmt.Errorf(
			"calendar object at path %q contains a component outside the calendar component set: %w",
			storePath,
			err,
		)
	}

	objPtr, err := caldavObjectToStoreObject(objectPath, calendar)
	if err != nil {
		return nil, false, fmt.Errorf(
			"could not convert caldav Object at path %q into store Object: %w",
			objectPath,
			err,
		)
	}

	obj := *objPtr

	created, err = b.store.PutCalendarObject(ctx, obj)
	if err != nil {
		return nil, false, fmt.Errorf("error while storing calendar object at path %q in storage: %w", objectPath, err)
	}

	object, err = storeObjectToCaldavObject(ctx, obj)
	if err != nil {
		return nil, false, fmt.Errorf("error while converting stored object back to a caldav CalendarObject: %w", err)
	}

	we.withCalendarObject(object)

	return object, created, nil
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
		err = fmt.Errorf("error while fetching calendar object at path %q from storage: %w", path, err)
		if errors.Is(err, store.ErrNotFound) {
			return nil, &daverr.HTTPError{Code: http.StatusNotFound, Err: err}
		}

		return nil, err
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

	we.Query = query

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

func (b *Backend) DeleteCalendarObject(
	ctx context.Context,
	path string,
	opts *caldav.DeleteCalendarObjectOptions,
) (err error) {
	ctx, we, handle := wideevent.Start[DeleteCalendarObjectWideEvent](ctx, "DeleteCalendarObject")
	defer func() { handle.End(err) }()

	if opts == nil {
		opts = &caldav.DeleteCalendarObjectOptions{}
	}

	storePath := normalizePath(path)
	we.Path = storePath

	if opts.IfMatch.IsSet() {
		we.MatchETag = string(opts.IfMatch)
	}

	if opts.IfNoneMatch.IsSet() {
		we.NoneMatchETag = string(opts.IfNoneMatch)
	}

	obj, err := b.store.GetCalendarObject(ctx, storePath)
	if err != nil {
		err = fmt.Errorf("error while fetching calendar object at path %q from storage: %w", path, err)
		if errors.Is(err, store.ErrNotFound) {
			if err := checkETagPreconditions(nil, false, opts.IfMatch, opts.IfNoneMatch); err != nil {
				return err
			}

			return &daverr.HTTPError{Code: http.StatusNotFound, Err: err}
		}

		return err
	}

	if err := checkETagPreconditions(obj, true, opts.IfMatch, opts.IfNoneMatch); err != nil {
		return err
	}

	if err := b.store.DeleteCalendarObject(ctx, storePath); err != nil {
		err = fmt.Errorf("error while deleting calendar object at path %q in storage: %w", path, err)
		if errors.Is(err, store.ErrNotFound) {
			return &daverr.HTTPError{Code: http.StatusNotFound, Err: err}
		}

		return err
	}

	return nil
}

func checkPutPreconditions(
	existing *store.Object,
	exists bool,
	opts *caldav.PutCalendarObjectOptions,
	we *PutCalendarObjectWideEvent,
) error {
	if opts == nil {
		return nil
	}

	if opts.IfMatch.IsSet() {
		we.MatchETag = string(opts.IfMatch)
	}

	if opts.IfNoneMatch.IsSet() {
		we.NoneMatchETag = string(opts.IfNoneMatch)
	}

	return checkETagPreconditions(existing, exists, opts.IfMatch, opts.IfNoneMatch)
}

func checkETagPreconditions(
	existing *store.Object,
	exists bool,
	ifMatch webdav.ConditionalMatch,
	ifNoneMatch webdav.ConditionalMatch,
) error {
	if ifMatch.IsSet() {
		if !exists {
			return &daverr.HTTPError{
				Code: http.StatusPreconditionFailed,
				Err:  ErrETagDidNotMatch,
			}
		}

		match, err := ETagMatcher(ifMatch.MatchETag)(existing.ETag)
		if err != nil {
			return &daverr.HTTPError{
				Code: http.StatusPreconditionFailed,
				Err:  fmt.Errorf("error while matching etag: %w", err),
			}
		}

		if !match {
			return &daverr.HTTPError{
				Code: http.StatusPreconditionFailed,
				Err:  ErrETagDidNotMatch,
			}
		}
	}

	if ifNoneMatch.IsSet() {
		if exists {
			match, err := NegateETagMatch(ETagMatcher(ifNoneMatch.MatchETagWeak))(existing.ETag)
			if err != nil {
				return &daverr.HTTPError{
					Code: http.StatusPreconditionFailed,
					Err:  fmt.Errorf("error while matching etag: %w", err),
				}
			}

			if !match {
				return &daverr.HTTPError{
					Code: http.StatusPreconditionFailed,
					Err:  ErrETagDidNotMatch,
				}
			}
		}
	}

	return nil
}

func calendarUID(calendar *ics.Calendar) string {
	for _, component := range calendar.Components {
		var uid string

		switch component := component.(type) {
		case *ics.VEvent:
			uid = component.Id()
		case *ics.VTodo:
			uid = component.Id()
		case *ics.VJournal:
			uid = component.Id()
		case *ics.VBusy:
			uid = component.Id()
		}

		if uid != "" {
			return uid
		}
	}

	return ""
}

// Ensure Backend implements caldav.Backend.
var _ caldav.Backend = &Backend{}
