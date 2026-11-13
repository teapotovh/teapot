package httpauth

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
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
	return append([]prometheus.Collector{
		ba.metrics.duration,
	}, ba.factory.Metrics()...)
}

// Metrics implements observability.Metrics.
func (ja *JWTAuth) Metrics() []prometheus.Collector {
	return append([]prometheus.Collector{
		ja.metrics.duration,
	}, ja.factory.Metrics()...)
}
