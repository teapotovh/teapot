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
	active       prometheus.Gauge
}

var httpSizeBucket = prometheus.ExponentialBuckets(100, 10, 5)

func (m *metrics) incActive() {
	m.active.Inc()
}

func (m *metrics) decActive() {
	m.active.Dec()
}

func (m *metrics) observeRequest(method, status string, duration time.Duration, requestSize, responseSize int) {
	m.total.WithLabelValues(method, status).Inc()
	m.duration.WithLabelValues(method).Observe(duration.Seconds())
	m.requestSize.WithLabelValues(method).Observe(float64(requestSize))
	m.responseSize.WithLabelValues(method).Observe(float64(responseSize))
}

func (h *HTTPSrv) metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		h.metrics.incActive()
		defer h.metrics.decActive()

		// Capture response status
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(rw, r)

		h.metrics.observeRequest(
			r.Method,
			strconv.Itoa(rw.statusCode),
			time.Since(start),
			int(r.ContentLength),
			rw.bytesWritten,
		)
	})
}

type responseWriter struct {
	http.ResponseWriter

	statusCode   int
	bytesWritten int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	if rw.bytesWritten <= 0 {
		rw.ResponseWriter.WriteHeader(code)
	}
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
		h.metrics.active,
	}
}
