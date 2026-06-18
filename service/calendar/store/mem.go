package store

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/teapotovh/teapot/lib/observability"
	"github.com/teapotovh/teapot/lib/run"
)

var (
	ErrCalendarAlreadyExists       = errors.New("calendar already exists")
	ErrMissingCalendar             = errors.New("missing calendar")
	ErrCalendarObjectAlreadyExists = errors.New("calendar object already exists")
	ErrMissingCalendarObject       = errors.New("missing calendar object")
)

type Mem struct {
	mu        sync.RWMutex
	calendars map[Path]Calendar
	objects   map[Path]Object

	metrics metrics
}

func NewMem() *Mem {
	m := Mem{
		calendars: map[Path]Calendar{},
		objects:   map[Path]Object{},
	}
	m.metrics.initMetrics("mem")

	return &m
}

// Ping implements Store.
func (m *Mem) Ping(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	return nil
}

// CreateCalendar implements Store.
func (m *Mem) CreateCalendar(ctx context.Context, calendar Calendar) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.calendars[calendar.Path]; exists {
		return ErrCalendarAlreadyExists
	}

	m.calendars[calendar.Path] = calendar

	return nil
}

// ListCalendars implements Store.
func (m *Mem) ListCalendars(ctx context.Context, basePath Path) ([]Calendar, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var calendars []Calendar
	for _, calendar := range m.calendars {
		if strings.HasPrefix(string(calendar.Path), string(basePath)) {
			calendars = append(calendars, calendar)
		}
	}

	return calendars, nil
}

// GetCalendar implements Store.
func (m *Mem) GetCalendar(ctx context.Context, path Path) (*Calendar, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if calendar, exists := m.calendars[path]; exists {
		return &calendar, nil
	}

	return nil, ErrMissingCalendar
}

// CreateCalendarObject implements Store.
func (m *Mem) CreateCalendarObject(ctx context.Context, object Object) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.objects[object.Path]; exists {
		return ErrCalendarObjectAlreadyExists
	}

	m.objects[object.Path] = object

	return nil
}

// ListCalendarObjects implements Store.
func (m *Mem) ListCalendarObjects(ctx context.Context, path Path) ([]Object, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var objects []Object
	for _, object := range m.objects {
		if strings.HasPrefix(string(object.Path), string(path)) {
			objects = append(objects, object)
		}
	}

	return objects, nil
}

// GetCalendarObject implements Store.
func (m *Mem) GetCalendarObject(ctx context.Context, path Path) (*Object, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if object, exists := m.objects[path]; exists {
		return &object, nil
	}

	return nil, ErrMissingCalendarObject
}

// DeleteCalendarObject implements Store.
func (m *Mem) DeleteCalendarObject(ctx context.Context, path Path) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.objects[path]; !exists {
		return ErrMissingCalendarObject
	}

	delete(m.objects, path)

	return nil
}

// Run implements run.Runnable
//
// This is a no-op.
func (m *Mem) Run(ctx context.Context, notify run.Notify) error {
	notify.Notify()
	return nil
}

// Metrics implements observability.Metrics.
func (m *Mem) Metrics() []prometheus.Collector {
	return []prometheus.Collector{m.metrics.backend}
}

// ReadinessChecks implements run.ReadinessChecks
//
// This is a no-op.
func (m *Mem) ReadinessChecks() map[string]observability.Check {
	return map[string]observability.Check{}
}
