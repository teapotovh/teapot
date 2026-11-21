package log

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	total    *prometheus.CounterVec
	duration *prometheus.HistogramVec
	size     *prometheus.GaugeVec
}

func (l *Log) initMetrics() {
	l.metrics.total = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "log_lines_total",
			Help: "Total number of log lines stored",
		},
		[]string{"source", "level"},
	)

	l.metrics.duration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "log_line_flush_duration",
			Help:    "Duration of how long a log line stays in the queue until it's flushed to disk",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"source", "level"},
	)

	l.metrics.size = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "log_file_size_total",
			Help: "Number of bytes in the current log file",
		},
		[]string{"source"},
	)
}

func (m *metrics) observeLine(source, level string, duration time.Duration) {
	m.total.WithLabelValues(source, level).Inc()
	m.duration.WithLabelValues(source, level).Observe(duration.Seconds())
}

func (m *metrics) setSize(source string, size uint64) {
	m.size.WithLabelValues(source).Set(float64(size))
}

func (l *Log) Metrics() []prometheus.Collector {
	return []prometheus.Collector{
		l.metrics.total,
		l.metrics.duration,

		l.metrics.size,
	}
}
