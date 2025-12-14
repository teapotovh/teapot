package kontakte

import (
	"context"
	"log/slog"
	"net/http"
)

type contextKey string

const logContextKey contextKey = "log"

func getLogger(ctx context.Context) *slog.Logger {
	return ctx.Value(logContextKey).(*slog.Logger)
}

func LogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := getAuth(r.Context())

		log := slog.Default()
		if auth != nil {
			log = log.With("user", auth.Subject, "admin", auth.Admin)
		}

		log.InfoContext(r.Context(), "received request", "method", r.Method, "url", r.URL)
		r = r.WithContext(context.WithValue(r.Context(), logContextKey, log))
		next.ServeHTTP(w, r)
	})
}
