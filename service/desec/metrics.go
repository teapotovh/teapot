package desec

import (
	"net/http"

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
