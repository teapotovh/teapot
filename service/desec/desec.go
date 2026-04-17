package desec

import (
	"fmt"
	"log/slog"
	"net/http"
	"slices"

	"github.com/teapotovh/teapot/lib/httplog"
	// webhookapi "sigs.k8s.io/external-dns/provider/webhook/api"
)

type Desec struct {
	logger *slog.Logger

	token  string
	domain string

	httpLog *httplog.HTTPLog
	metrics metrics
}

// DesecConfig is the configuration for the Desec service.
type DesecConfig struct {
	Token  string
	Domain string

	HTTPLog httplog.HTTPLogConfig
}

func NewDesec(config DesecConfig, logger *slog.Logger) (*Desec, error) {
	// Provide request information in all log operations
	logger = httplog.WithHandler(logger)

	httpLog, err := httplog.NewHTTPLog(config.HTTPLog, logger.With("component", "httplog"))
	if err != nil {
		return nil, fmt.Errorf("error while constructing httplog: %w", err)
	}

	desec := Desec{
		logger: logger,

		token:  config.Token,
		domain: config.Domain,

		httpLog: httpLog,
	}

	desec.initMetrics()

	return &desec, nil
}

func allowMethod(handler http.HandlerFunc, methods ...string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !slices.Contains(methods, r.Method) {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		handler(w, r)
	})
}

var (
	UrlAdjustEndpoints = "/adjustendpoints"
	UrlApplyChanges    = "/applychanges"
	UrlRecords         = "/records"
)

type srv struct {
	logger *slog.Logger
}

func (s *srv) NegotiateHandler(w http.ResponseWriter, r *http.Request) {
	s.logger.InfoContext(r.Context(), "dio cane 1")
	w.Write([]byte("NegotiateHandler"))
}

func (s *srv) RecordsHandler(w http.ResponseWriter, r *http.Request) {
	s.logger.InfoContext(r.Context(), "dio cane 2")
	w.Write([]byte("RecordsHandler"))
}

func (s *srv) AdjustEndpointsHandler(w http.ResponseWriter, r *http.Request) {
	s.logger.InfoContext(r.Context(), "dio cane 3")
	w.Write([]byte("AdjustEndpointsHandler"))
}

func (d *Desec) Handler(prefix string) http.Handler {
	srv := srv{d.logger}
	// srv := webhookapi.WebhookServer{
	// 	Provider:
	// }

	mux := http.NewServeMux()

	mux.Handle("/", allowMethod(srv.NegotiateHandler, http.MethodGet))
	mux.Handle(UrlRecords, allowMethod(srv.RecordsHandler, http.MethodGet, http.MethodPost))
	mux.Handle(UrlAdjustEndpoints, allowMethod(srv.AdjustEndpointsHandler, http.MethodPost))

	var handler http.Handler = mux

	handler = d.httpLog.LogMiddleware(handler)
	handler = d.httpLog.ExtractMiddleware(handler)

	return handler
}
