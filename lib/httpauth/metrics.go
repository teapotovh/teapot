package httpauth

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/teapotovh/teapot/lib/observability"
)

type metrics struct {
	duration *prometheus.HistogramVec
}

const namespace = "httpauth"

const (
	metricsMethodBasic = "basic"
	metricsMethodJWT   = "jwt"

	metricsStatusSuccess = "success"
	metricsStatusInvalid = "invalid"
	metricsStatusError   = "error"
)

func (m *metrics) init() {
	m.duration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "authentication_duration_seconds",
			Help:      "Authentication attempt latency in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"method", "status"},
	)
}

func (m *metrics) observe(method, status string, duration time.Duration) {
	m.duration.WithLabelValues(method, status).Observe(duration.Seconds())
}

// Metrics implements observability.Metrics.
func (ba *BasicAuth) Metrics() []prometheus.Collector {
	collectors := []prometheus.Collector{ba.metrics.duration}

	if m, ok := ba.authenticator.(observability.Metrics); ok {
		collectors = append(collectors, m.Metrics()...)
	}

	return collectors
}

// Metrics implements observability.Metrics.
func (ja *JWTAuth) Metrics() []prometheus.Collector {
	collectors := []prometheus.Collector{ja.metrics.duration}

	if m, ok := ja.authenticator.(observability.Metrics); ok {
		collectors = append(collectors, m.Metrics()...)
	}

	return collectors
}
