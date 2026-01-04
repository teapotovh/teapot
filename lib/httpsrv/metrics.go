package httpsrv

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// TODO: this is a big change, not sure how to implement it.
// We should add a "path" label to each metric, but it should not be the
// actual request path, but rather the path that the handler was registered for.

type metrics struct {
	total        *prometheus.CounterVec
	duration     *prometheus.HistogramVec
	requestSize  *prometheus.HistogramVec
	responseSize *prometheus.HistogramVec
	inFlight     prometheus.Gauge
}

var httpSizeBucket = prometheus.ExponentialBuckets(100, 10, 5)

func (h *HTTPSrv) initMetrics() {
	// Request metrics
	h.metrics.total = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "httpsrv_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "status"},
	)

	h.metrics.duration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "httpsrv_request_duration_seconds",
			Help:    "HTTP request latency in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method"},
	)

	h.metrics.requestSize = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "httpsrv_request_size_bytes",
			Help:    "HTTP request size in bytes",
			Buckets: httpSizeBucket,
		},
		[]string{"method"},
	)

	h.metrics.responseSize = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "httpsrv_response_size_bytes",
			Help:    "HTTP response size in bytes",
			Buckets: httpSizeBucket,
		},
		[]string{"method"},
	)

	h.metrics.inFlight = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "httpsrv_requests_in_flight",
			Help: "Current number of HTTP requests being served",
		},
	)
}

func (h *HTTPSrv) metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		h.metrics.inFlight.Inc()
		defer h.metrics.inFlight.Dec()

		// Capture response status
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(rw, r)

		duration := time.Since(start).Seconds()
		method := r.Method
		status := strconv.Itoa(rw.statusCode)

		h.metrics.total.WithLabelValues(method, status).Inc()
		h.metrics.duration.WithLabelValues(method).Observe(duration)
		h.metrics.requestSize.WithLabelValues(method).Observe(float64(r.ContentLength))
		h.metrics.responseSize.WithLabelValues(method).Observe(float64(rw.bytesWritten))
	})
}

type responseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += n
	return n, err
}

// Metrics implements observability.Metrics.
func (h *HTTPSrv) Metrics() []prometheus.Collector {
	return []prometheus.Collector{
		h.metrics.total,
		h.metrics.duration,
		h.metrics.requestSize,
		h.metrics.responseSize,
		h.metrics.inFlight,
	}
}
