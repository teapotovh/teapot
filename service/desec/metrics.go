package desec

import (
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
		[]string{"code"},
	)

	d.metrics.duration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "desec_calls_duration",
			Help:    "deSEC API calls latency",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"code"},
	)

	d.metrics.providerTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "desec_provider_calls_total",
			Help: "Total number external-dns provider calls",
		},
		[]string{"code"},
	)

	d.metrics.providerDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "desec_provider_calls_duration",
			Help:    "external-dns API call latency",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"code"},
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
