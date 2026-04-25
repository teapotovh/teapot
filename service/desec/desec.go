package desec

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/nrdcg/desec"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/teapotovh/teapot/lib/httplog"
	"github.com/teapotovh/teapot/lib/log"
	ednsprovider "sigs.k8s.io/external-dns/provider"
)

type Desec struct {
	logger *slog.Logger

	domain string

	httpLog  *httplog.HTTPLog
	client   *desec.Client
	provider ednsprovider.Provider
	webhook  *webhook
	metrics  metrics
}

// DesecConfig is the configuration for the Desec service.
type DesecConfig struct {
	Token      string
	Domain     string
	MaxRetries uint64
	DryRun     bool

	HTTPLog httplog.HTTPLogConfig
}

func NewDesec(config DesecConfig, logger *slog.Logger) (*Desec, error) {
	// Provide request information in all log operations
	logger = httplog.WithHandler(logger)

	httpLog, err := httplog.NewHTTPLog(config.HTTPLog, logger.With("component", "httplog"))
	if err != nil {
		return nil, fmt.Errorf("error while constructing httplog: %w", err)
	}

	clientOptions := desec.ClientOptions{
		RetryMax: int(config.MaxRetries),
		Logger:   log.NewRetryableHTTPAdaptor(logger.With("component", "client")),
	}
	if config.DryRun {
		clientOptions.HTTPClient = &http.Client{
			Transport: &MockTransport{
				logger: logger.With("component", "mock"),
			},
		}
	}
	client := desec.New(config.Token, clientOptions)

	desec := Desec{
		logger: logger,

		domain: config.Domain,

		httpLog: httpLog,
		client:  client,
	}

	desec.provider = &provider{
		logger: logger.With("component", "provider"),
		desec:  &desec,
	}

	desec.webhook = &webhook{
		logger:   logger.With("component", "webhook"),
		provider: desec.provider,
	}

	desec.initMetrics()

	return &desec, nil
}

func (d *Desec) collectWebhookMetrics(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := responseWriter{
			ResponseWriter: w,
		}
		handler.ServeHTTP(&rw, r)

		duration := time.Since(start)
		code := rw.statusCode
		labels := prometheus.Labels{
			"path": r.URL.Path,
			"code": strconv.Itoa(code),
		}

		d.metrics.providerTotal.With(labels).Inc()
		d.metrics.providerDuration.With(labels).Observe(duration.Seconds())
	})
}

func (d *Desec) Handler(prefix string) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc(UrlNegotiate, d.webhook.NegotiateHandler)
	mux.HandleFunc(UrlRecords, d.webhook.RecordsHandler)
	mux.HandleFunc(UrlAdjustEndpoints, d.webhook.AdjustEndpointsHandler)

	var handler http.Handler = mux

	handler = d.httpLog.LogMiddleware(handler)
	handler = d.httpLog.ExtractMiddleware(handler)
	handler = d.collectWebhookMetrics(handler)

	return handler
}
