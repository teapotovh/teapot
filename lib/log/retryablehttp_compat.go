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

// Error implements retryablehttp.LeveledLogger.
func (a *RetryableHTTPAdaptor) Error(msg string, keysAndValues ...any) {
	a.logger.Error(msg, keysAndValues...) //nolint:sloglint
}

// Info implements retryablehttp.LeveledLogger.
func (a *RetryableHTTPAdaptor) Info(msg string, keysAndValues ...any) {
	a.logger.Info(msg, keysAndValues...) //nolint:sloglint
}

// Debug implements retryablehttp.LeveledLogger.
func (a *RetryableHTTPAdaptor) Debug(msg string, keysAndValues ...any) {
	a.logger.Debug(msg, keysAndValues...) //nolint:sloglint
}

// Warn implements retryablehttp.LeveledLogger.
func (a *RetryableHTTPAdaptor) Warn(msg string, keysAndValues ...any) {
	a.logger.Warn(msg, keysAndValues...) //nolint:sloglint
}

// Ensure RetryableHTTPAdaptor implements retryablehttp.LeveledLogger.
var _ retryablehttp.LeveledLogger = &RetryableHTTPAdaptor{}
