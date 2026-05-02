package log

import "github.com/prometheus/client_golang/prometheus"

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
			Name: "log_file_size",
			Help: "Number of bytes in the current log file",
		},
		[]string{"source"},
	)
}

func (l *Log) Metrics() []prometheus.Collector {
	// TODO
	return []prometheus.Collector{
		l.metrics.total,
		l.metrics.duration,

		l.metrics.size,
	}
}
