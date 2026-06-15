package store

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	ErrCalendarAlreadyExists       = errors.New("calendar already exists")
	ErrMissingCalendar             = errors.New("missing calendar")
	ErrCalendarObjectAlreadyExists = errors.New("calendar object already exists")
	ErrMissingCalendarObject       = errors.New("missing calendar object")
)

type Mem struct {
	mu        sync.RWMutex
	calendars map[string]Calendar
	objects   map[string]Object

	metrics metrics
}

func NewMem() *Mem {
	m := Mem{
		calendars: map[string]Calendar{},
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
func (m *Mem) CreateCalendar(ctx context.Context, calendar Calendar) (err error) {
	start := time.Now()

	defer func() {
		m.metrics.operationDuration.WithLabelValues(operationCreateCalendar, status(err)).
			Observe(time.Since(start).Seconds())
	}()

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.calendars[calendar.Path]; exists {
		return ErrCalendarAlreadyExists
	}

	m.calendars[calendar.Path] = calendar

	return nil
}

// ListCalendars implements Store.
func (m *Mem) ListCalendars(ctx context.Context, basePath string) (calendars []Calendar, err error) {
	start := time.Now()

	defer func() {
		m.metrics.operationDuration.WithLabelValues(operationCreateCalendar, status(err)).
			Observe(time.Since(start).Seconds())
	}()

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, calendar := range m.calendars {
		if strings.HasPrefix(calendar.Path, basePath) {
			calendars = append(calendars, calendar)
		}
	}

	return calendars, nil
}

// GetCalendar implements Store.
func (m *Mem) GetCalendar(ctx context.Context, path string) (calendar *Calendar, err error) {
	start := time.Now()

	defer func() {
		m.metrics.operationDuration.WithLabelValues(operationGetCalendar, status(err)).
			Observe(time.Since(start).Seconds())
	}()

	m.mu.Lock()
	defer m.mu.Unlock()

	if calendar, exists := m.calendars[path]; exists {
		return &calendar, nil
	}

	return nil, ErrMissingCalendar
}

// CreateCalendarObject implements Store.
func (m *Mem) CreateCalendarObject(ctx, object Object) (err error) {
	start := time.Now()

	defer func() {
		m.metrics.operationDuration.WithLabelValues(operationCreateCalendarObject, status(err)).
			Observe(time.Since(start).Seconds())
	}()

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.objects[object.Path]; exists {
		return ErrCalendarObjectAlreadyExists
	}

	m.objects[object.Path] = object

	return nil
}

// ListCalendarObjects implements Store.
func (m *Mem) ListCalendarObjects(ctx, path string) (objects []Object, err error) {
	start := time.Now()

	defer func() {
		m.metrics.operationDuration.WithLabelValues(operationListCalendarObjects, status(err)).
			Observe(time.Since(start).Seconds())
	}()

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, object := range m.objects {
		if strings.HasPrefix(object.Path, path) {
			objects = append(objects, object)
		}
	}

	return objects, nil
}

// GetCalendarObject implements Store.
func (m *Mem) GetCalendarObject(ctx context.Context, path string) (object *Object, err error) {
	start := time.Now()

	defer func() {
		m.metrics.operationDuration.WithLabelValues(operationGetCalendarObject, status(err)).
			Observe(time.Since(start).Seconds())
	}()

	m.mu.Lock()
	defer m.mu.Unlock()

	if object, exists := m.objects[path]; exists {
		return &object, nil
	}

	return nil, ErrMissingCalendarObject
}

// DeleteCalendarObject implements Store.
func (m *Mem) DeleteCalendarObject(ctx, path string) (err error) {
	start := time.Now()

	defer func() {
		m.metrics.operationDuration.WithLabelValues(operationDeleteCalendarObject, status(err)).
			Observe(time.Since(start).Seconds())
	}()

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.objects[path]; !exists {
		return ErrMissingCalendarObject
	}

	delete(m.objects, path)

	return nil
}

// Metrics implements observability.Metrics.
func (m *Mem) Metrics() []prometheus.Collector {
	return []prometheus.Collector{
		m.metrics.backend,
		m.metrics.operationDuration,
		m.metrics.transactionDuration,
	}
}
