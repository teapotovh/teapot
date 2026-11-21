package pagedbtree

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	path string

	entrySize         *prometheus.HistogramVec
	entries           *prometheus.GaugeVec
	cachedEntries     *prometheus.GaugeVec
	operationDuration *prometheus.HistogramVec
}

const (
	metricsNamespace = "pagedbtree"

	metricsOperationLoad      = "load"
	metricsOperationStore     = "store"
	metricsOperationDelete    = "delete"
	metricsOperationAll       = "all"
	metricsOperationFrom      = "from"
	metricsOperationBetween   = "between"
	metricsOperationWriteback = "writeback"
)

var entrySizeBuckets = prometheus.ExponentialBuckets(64, 4, 10)

func (m *metrics) init(path string) {
	m.path = path

	m.entrySize = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Name:      "entry_size_bytes",
			Help:      "Size of entries written to the store in bytes",
			Buckets:   entrySizeBuckets,
		},
		[]string{"path"},
	)

	m.entries = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Name:      "entries",
			Help:      "Total number of entries in the tree",
		},
		[]string{"path"},
	)

	m.cachedEntries = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Name:      "cached_entries",
			Help:      "Number of entries in pages in the page cache",
		},
		[]string{"path"},
	)

	m.operationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Name:      "operation_duration_seconds",
			Help:      "Duration of store operations in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"path", "operation"},
	)
}

func (m *metrics) collectors() []prometheus.Collector {
	return []prometheus.Collector{
		m.entrySize,
		m.entries,
		m.cachedEntries,
		m.operationDuration,
	}
}

func (m *metrics) observeDuration(operation string, start time.Time) {
	m.operationDuration.WithLabelValues(m.path, operation).Observe(time.Since(start).Seconds())
}

func (m *metrics) observeEntrySize(size int) {
	m.entrySize.WithLabelValues(m.path).Observe(float64(size))
}

func (m *metrics) setEntries(total int64) {
	m.entries.WithLabelValues(m.path).Set(float64(total))
}

func (m *metrics) incEntries() {
	m.entries.WithLabelValues(m.path).Inc()
}

func (m *metrics) decEntries() {
	m.entries.WithLabelValues(m.path).Dec()
}

func (m *metrics) setCachedEntries(total int64) {
	m.cachedEntries.WithLabelValues(m.path).Set(float64(total))
}

// Metrics implements observability.Metrics.
func (b *Bytes) Metrics() []prometheus.Collector {
	return b.metrics.collectors()
}
