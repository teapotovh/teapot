package log

import (
	"log/slog"

	"github.com/hashicorp/go-retryablehttp"
)

type RetryableHTTPAdaptor struct {
	logger *slog.Logger
}

func NewRetryableHTTPAdaptor(logger *slog.Logger) *RetryableHTTPAdaptor {
	return &RetryableHTTPAdaptor{
		logger: logger,
	}
}

// Error implements retryablehttp.LeveledLogger
func (a *RetryableHTTPAdaptor) Error(msg string, keysAndValues ...interface{}) {
	a.logger.Error(msg, keysAndValues...)
}

// Info implements retryablehttp.LeveledLogger
func (a *RetryableHTTPAdaptor) Info(msg string, keysAndValues ...interface{}) {
	a.logger.Info(msg, keysAndValues...)
}

// Debug implements retryablehttp.LeveledLogger
func (a *RetryableHTTPAdaptor) Debug(msg string, keysAndValues ...interface{}) {
	a.logger.Debug(msg, keysAndValues...)
}

// Warn implements retryablehttp.LeveledLogger
func (a *RetryableHTTPAdaptor) Warn(msg string, keysAndValues ...interface{}) {
	a.logger.Warn(msg, keysAndValues...)
}

// Ensure RetryableHTTPAdaptor implements retryablehttp.LeveledLogger
var _ retryablehttp.LeveledLogger = &RetryableHTTPAdaptor{}
