package observability

import (
	"log/slog"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type httpServicePrometheus struct {
	logger *slog.Logger

	registry *prometheus.Registry
}

// promLogger is an adapter for the error handling logger in promhttp
type promLogger struct {
	logger *slog.Logger
}

func (l promLogger) Println(v ...any) {
	l.logger.Error("error while performing a metrics export", "args", v)
}

// Handler implements httpsrv.Handler
func (p *httpServicePrometheus) Handler(prefix string) http.Handler {
	pl := promLogger{
		logger: p.logger.With("component", "prometheus"),
	}

	return promhttp.HandlerFor(p.registry, promhttp.HandlerOpts{
		ErrorLog: pl,
		Registry: p.registry,
	})
}
