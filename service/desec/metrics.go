package desec

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	total    *prometheus.CounterVec
	duration *prometheus.HistogramVec

	providerTotal    *prometheus.CounterVec
	providerDuration *prometheus.HistogramVec
}

func (d *Desec) initMetrics() {
	d.metrics.total = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "desec_calls_total",
			Help: "Total number of deSEC API calls",
		},
		[]string{"operation", "error"},
	)

	d.metrics.duration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "desec_calls_duration",
			Help:    "deSEC API calls latency",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"operation", "error"},
	)

	d.metrics.providerTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "desec_provider_calls_total",
			Help: "Total number external-dns provider calls",
		},
		[]string{"path", "code"},
	)

	d.metrics.providerDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "desec_provider_calls_duration",
			Help:    "external-dns API call latency",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"path", "code"},
	)
}

func (m *metrics) observeCall(operation, hasError string, duration time.Duration) {
	labels := prometheus.Labels{
		"operation": operation,
		"error":     hasError,
	}

	m.total.With(labels).Inc()
	m.duration.With(labels).Observe(duration.Seconds())
}

func (m *metrics) observeProviderCall(path, code string, duration time.Duration) {
	labels := prometheus.Labels{
		"path": path,
		"code": code,
	}

	m.providerTotal.With(labels).Inc()
	m.providerDuration.With(labels).Observe(duration.Seconds())
}

// Metrics implements observability.Metrics.
func (d *Desec) Metrics() []prometheus.Collector {
	return []prometheus.Collector{
		d.metrics.total,
		d.metrics.duration,

		d.metrics.providerTotal,
		d.metrics.providerDuration,
	}
}

type responseWriter struct {
	http.ResponseWriter

	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
