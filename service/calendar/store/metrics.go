package store

import (
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	backend             *prometheus.CounterVec
	operationDuration   *prometheus.HistogramVec
	transactionDuration *prometheus.HistogramVec
}

const (
	statusSuccess = "success"
	statusError   = "error"

	operationCreateCalendar = "create_calendar"
	operationListCalendars  = "list_calendars"
	operationGetCalendar    = "get_calendar"

	operationCreateCalendarObject = "create_calendar_object"
	operationListCalendarObjects  = "list_calendar_objects"
	operationGetCalendarObject    = "get_calendar_object"
	operationDeleteCalendarObject = "delete_calendar_object"
)

func status(err error) string {
	if err != nil {
		return statusError
	}

	return statusSuccess
}

func (m *metrics) initMetrics(backend string) {
	m.backend = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "calendar_store_backend_total",
			Help: "Number of backends used for calendar storage",
		},
		[]string{"backend"},
	)

	// This is used purely to track which backend the current bottin instance is running
	m.backend.WithLabelValues(backend).Add(1)

	m.operationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "calendar_store_operation_duration",
			Help:    "Duration of how long a store operation took",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"operation", "status"},
	)
}
