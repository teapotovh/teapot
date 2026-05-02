package log

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/teapotovh/teapot/lib/httphandler"
	"github.com/teapotovh/teapot/lib/httplog"
	"github.com/teapotovh/teapot/lib/run"
)

type Log struct {
	logger *slog.Logger

	path    string
	manager *Manager

	httpHandler *httphandler.HTTPHandler
	httpLog     *httplog.HTTPLog
	metrics     metrics
}

type LogConfig struct {
	Path     string
	Capacity uint32

	HTTPHandler httphandler.HTTPHandlerConfig
	HTTPLog     httplog.HTTPLogConfig
}

func NewLog(config LogConfig, logger *slog.Logger) (*Log, error) {
	httpHandler := httphandler.NewHTTPHandler(
		config.HTTPHandler,
		httphandler.DefaultErrorHandlers,
		logger.With("component", "httphandler"),
	)

	httpLog, err := httplog.NewHTTPLog(config.HTTPLog, logger.With("component", "httplog"))
	if err != nil {
		return nil, fmt.Errorf("error while constructing httplog: %w", err)
	}

	manager := NewManager(config.Capacity, logger.With("component", "manager"))

	log := Log{
		logger: logger,

		path:    config.Path,
		manager: manager,

		httpHandler: httpHandler,
		httpLog:     httpLog,
	}

	log.initMetrics()

	return &log, nil
}

// Run implements run.Runnable.
func (l *Log) Run(ctx context.Context, notify run.Notify) (err error) {
	return l.manager.Run(ctx, notify)
}

func (l *Log) Handler(prefix string) http.Handler {
	// TODO
	mux := http.NewServeMux()

	mux.Handle(URLLogs, l.httpHandler.Adapt(l.handleLogs))

	var handler http.Handler = mux

	handler = l.httpLog.LogMiddleware(handler)
	handler = l.httpLog.ExtractMiddleware(handler)

	return handler
}
