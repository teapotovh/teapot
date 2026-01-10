package alertmanager

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/teapotovh/teapot/lib/httphandler"
	"github.com/teapotovh/teapot/lib/httplog"
	"github.com/teapotovh/teapot/service/alert"
)

const PathWebHook = "/webhook"

type AlertManager struct {
	logger *slog.Logger

	httpLog     *httplog.HTTPLog
	httpHandler *httphandler.HTTPHandler
	metrics     metrics
}

type AlertManagerConfig struct {
	HTTPLog     httplog.HTTPLogConfig
	HTTPHandler httphandler.HTTPHandlerConfig
}

func NewAlertManager(alert *alert.Alert, config AlertManagerConfig, logger *slog.Logger) (*AlertManager, error) {
	// Provide request information in all log operations
	logger = httplog.WithHandler(logger)

	httplog, err := httplog.NewHTTPLog(config.HTTPLog, logger.With("component", "httplog"))
	if err != nil {
		return nil, fmt.Errorf("error while initializing component httplog: %w", err)
	}

	httpHandler := httphandler.NewHTTPHandler(
		config.HTTPHandler,
		httphandler.DefaultErrorHandlers,
		logger.With("component", "httphandler"),
	)

	am := AlertManager{
		logger: logger,

		httpLog:     httplog,
		httpHandler: httpHandler,
	}

	am.initMetrics()

	return &am, nil
}

// Handler implements httpsrv.HTTPService.
func (am *AlertManager) Handler(prefix string) http.Handler {
	mux := http.NewServeMux()

	mux.Handle(PathWebHook, am.httpHandler.Adapt(am.Webhook))

	var handler http.Handler = mux

	handler = am.httpLog.LogMiddleware(handler)
	handler = am.httpLog.ExtractMiddleware(handler)

	return handler
}
