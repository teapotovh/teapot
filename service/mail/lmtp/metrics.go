package lmtp

import (
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	messages *prometheus.CounterVec
}

func (m *metrics) initMetrics() {
	m.messages = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mail_lmtp_messages_total",
			Help: "Total number of LMTP message deliveries",
		},
		[]string{"status"},
	)
	m.messages.WithLabelValues(statusSuccess).Add(0)
	m.messages.WithLabelValues(statusError).Add(0)
}

func (m *metrics) observe(err error) {
	status := statusSuccess
	if err != nil {
		status = statusError
	}

	m.messages.WithLabelValues(status).Inc()
}
