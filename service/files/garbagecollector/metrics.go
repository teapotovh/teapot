package garbagecollector

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	metricsNamespace = "files"
	metricsSubsystem = "garbagecollector"
)

type metrics struct {
	deletedObjects prometheus.Counter
	errors         prometheus.Counter
	runDuration    prometheus.Histogram
}

func (m *metrics) initMetrics() {
	m.deletedObjects = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsSubsystem,
		Name:      "deleted_objects_total",
		Help:      "Total number of deleted S3 objects",
	})

	m.errors = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsSubsystem,
		Name:      "errors_total",
		Help:      "Total number of garbage collector errors",
	})

	m.runDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsSubsystem,
		Name:      "run_duration_seconds",
		Help:      "Duration of a garbage collector run in seconds",
		Buckets:   prometheus.DefBuckets,
	})
}

func (m *metrics) incDeletedObjects() {
	m.deletedObjects.Inc()
}

func (m *metrics) incErrors() {
	m.errors.Inc()
}

func (m *metrics) observeRunDuration(start time.Time) {
	m.runDuration.Observe(time.Since(start).Seconds())
}

// Metrics implements observability.Metrics.
func (c *GarbageCollector) Metrics() []prometheus.Collector {
	return []prometheus.Collector{
		c.metrics.deletedObjects,
		c.metrics.errors,
		c.metrics.runDuration,
	}
}
