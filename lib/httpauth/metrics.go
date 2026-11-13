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
// Only the httpauth-owned collectors are returned; metrics from the
// underlying LDAP factory are owned and exposed by its direct consumer.
func (ba *BasicAuth) Metrics() []prometheus.Collector {
	return []prometheus.Collector{ba.metrics.duration}
}

// Metrics implements observability.Metrics.
// Only the httpauth-owned collectors are returned; metrics from the
// underlying LDAP factory are owned and exposed by its direct consumer.
func (ja *JWTAuth) Metrics() []prometheus.Collector {
	return []prometheus.Collector{ja.metrics.duration}
}
