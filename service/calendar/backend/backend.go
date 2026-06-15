package backend

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/emersion/go-ical"

	"github.com/teapotovh/teapot/lib/webdav/caldav"
	"github.com/teapotovh/teapot/service/calendar/store"
)

var (
	errUnimplemented         = errors.New("operation not implemented")
	ErrUnexpectedNilCalendar = errors.New("unexpected nil calendar")

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

func (b *Backend) CreateCalendar(ctx context.Context, calendar *caldav.Calendar) error {
	b.logger.InfoContext(ctx, "called", "method", "CreateCalendar", "calendar", calendar)

	if calendar == nil {
		return ErrUnexpectedNilCalendar
	}

	err := b.store.CreateCalendar(ctx, store.Calendar{
		Path:        calendar.Path,
		Name:        calendar.Name,
		Description: calendar.Description,
	})
	if err != nil {
		return fmt.Errorf("error while creating calendar under path %q in storage: %w", calendar.Path, err)
	}

	return nil
}

func storeCalendarToCaldavCalendar(cal store.Calendar) caldav.Calendar {
	return caldav.Calendar{
		Path:        cal.Path,
		Name:        cal.Name,
		Description: cal.Description,

		MaxResourceSize:       MaxResourceSize,
		SupportedComponentSet: SupportedComponentSet,
	}
}

func (b *Backend) ListCalendars(ctx context.Context) ([]caldav.Calendar, error) {
	b.logger.InfoContext(ctx, "called", "method", "ListCalendars")

	path, err := b.CalendarHomeSetPath(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not get home-set path: %w", err)
	}

	cals, err := b.store.ListCalendars(ctx, path)
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

	cal, err := b.store.GetCalendar(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("error while getting calendar under path %q in storage: %w", path, err)
	}

	calendar := storeCalendarToCaldavCalendar(*cal)

	return &calendar, nil
}

func (b *Backend) PutCalendarObject(
	ctx context.Context,
	path string,
	calendar *ical.Calendar,
	opts *caldav.PutCalendarObjectOptions,
) (*caldav.CalendarObject, error) {
	b.logger.InfoContext(ctx, "called", "method", "PutCalendarObject", "path", path, "calendar", calendar, "opts", opts)

	return nil, fmt.Errorf("put calendar object: %w", errUnimplemented)
}

func (b *Backend) GetCalendarObject(
	ctx context.Context,
	path string,
	req *caldav.CalendarCompRequest,
) (*caldav.CalendarObject, error) {
	b.logger.InfoContext(ctx, "called", "method", "GetCalendarObject", "path", path, "req", req)

	return nil, fmt.Errorf("get calendar object %qv: %w", path, req, errUnimplemented)
}

func (b *Backend) ListCalendarObjects(
	ctx context.Context,
	path string,
	req *caldav.CalendarCompRequest,
) ([]caldav.CalendarObject, error) {
	return nil, fmt.Errorf("list calendar objects: %w", errUnimplemented)
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
