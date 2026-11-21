package pgcache

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	approachGet     = "get"
	approachList    = "list"
	approachFrom    = "from"
	approachBetween = "between"

	statusSuccess = "success"
	statusError   = "error"

	operationList   = "list"
	operationGet    = "get"
	operationStore  = "store"
	operationDelete = "delete"
)

func status(err error) string {
	if err != nil {
		return statusError
	}

	return statusSuccess
}

type metrics struct {
	objectsTotal               *prometheus.CounterVec
	loadsTotal                 *prometheus.CounterVec
	notificationUpdateDuration *prometheus.HistogramVec
	operationDuration          *prometheus.HistogramVec
	transactionDuration        *prometheus.HistogramVec
}

var tableMetrics = metrics{
	objectsTotal: prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pgcache_table_objects_total",
			Help: "Total number of objects in memory",
		},
		[]string{"table"},
	),
	loadsTotal: prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pgcache_table_loads_total",
			Help: "Total number of objects read from memory",
		},
		[]string{"table", "approach"},
	),
	notificationUpdateDuration: prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "pgcache_table_notification_duration_seconds",
			Help:    "Duration of the update for each notifications received",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"table"},
	),
	operationDuration: prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "pgcache_table_operation_duration_seconds",
			Help:    "Duration of how long a store operation took",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"table", "operation", "status"},
	),
	transactionDuration: prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "pgcache_table_transaction_duration_seconds",
			Help:    "Duration of how long a whole store transaction took",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"table", "operations", "status"},
	),
}

func (m *metrics) incObjects(table string) {
	m.objectsTotal.WithLabelValues(table).Inc()
}

func (m *metrics) decObjects(table string) {
	m.objectsTotal.WithLabelValues(table).Desc()
}

func (m *metrics) incLoads(table, approach string) {
	m.loadsTotal.WithLabelValues(table, approach).Inc()
}

func (m *metrics) observeOperation(table, operation string, err error, duration time.Duration) {
	m.operationDuration.WithLabelValues(table, operation, status(err)).Observe(duration.Seconds())
}

func (m *metrics) observeNotificationUpdate(table string, duration time.Duration) {
	m.notificationUpdateDuration.WithLabelValues(table).Observe(duration.Seconds())
}
