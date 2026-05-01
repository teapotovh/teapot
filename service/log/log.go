package log

import (
	"log/slog"
	"net/http"
)

type Log struct {
	logger *slog.Logger
}

type LogConfig struct {
}

func NewLog(config LogConfig, logger *slog.Logger) (*Log, error) {
	return &Log{
		logger: logger,
	}, nil
}

func (l *Log) Handler(prefix string) http.Handler {
	// TODO
	mux := http.NewServeMux()

	var handler http.Handler = mux

	return handler
}
