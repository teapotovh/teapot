package httpauth

import "github.com/prometheus/client_golang/prometheus"

// Metrics implements observability.Metrics.
func (ba *BasicAuth) Metrics() []prometheus.Collector {
	// TODO: export more metrics. See #34
	return ba.factory.Metrics()
}

// Metrics implements observability.Metrics.
func (ja *JWTAuth) Metrics() []prometheus.Collector {
	// TODO: export more metrics. See #34
	return ja.factory.Metrics()
}
