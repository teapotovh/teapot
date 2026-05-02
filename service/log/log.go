package log

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/teapotovh/teapot/lib/httphandler"
	"github.com/teapotovh/teapot/lib/httplog"
)

type Log struct {
	logger *slog.Logger

	path string

	httpHandler *httphandler.HTTPHandler
	httpLog     *httplog.HTTPLog
	metrics     metrics
}

type LogConfig struct {
	Path string

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

	log := Log{
		logger: logger,

		path: config.Path,

		httpHandler: httpHandler,
		httpLog:     httpLog,
	}

	log.initMetrics()

	return &log, nil
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
