package httplog

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/kataras/requestid"

	"github.com/teapotovh/teapot/lib/log"
)

type HTTPLogConfig struct {
	Level  string
	Ignore string
}

type HTTPLog struct {
	logger *slog.Logger

	level  slog.Level
	ignore *regexp.Regexp
}

func NewHTTPLog(config HTTPLogConfig, logger *slog.Logger) (*HTTPLog, error) {
	level, err := log.ParaseLogLevel(config.Level)
	if err != nil {
		return nil, err
	}

	re, err := regexp.Compile(config.Ignore)
	if err != nil {
		return nil, fmt.Errorf("error while parsing ignore regexp %q: %w", config.Ignore, err)
	}

	hl := HTTPLog{
		logger: logger,
		level:  level,
		ignore: re,
	}

	return &hl, nil
}

func (hl *HTTPLog) ExtractMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		ctx = context.WithValue(ctx, RequestID, requestid.Get(r))
		ctx = context.WithValue(ctx, RequestMethod, r.Method)
		ctx = context.WithValue(ctx, RequestURI, r.URL.RequestURI())

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (hl *HTTPLog) LogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ignore := hl.ignore.Match([]byte(r.URL.Path))

		var start time.Time
		if !ignore {
			start = time.Now()
		}

		next.ServeHTTP(w, r)

		if !ignore {
			duration := time.Since(start)
			hl.logger.Log(r.Context(), hl.level, "handled request", "elapsed", duration)
		}
	})
}
