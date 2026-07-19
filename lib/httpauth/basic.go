package httpauth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/teapotovh/teapot/lib/ldap"
	"github.com/teapotovh/teapot/lib/observability"
)

const (
	basicAuthErrContextKey contextKey = "basic-auth-err"
)

func GetBasicAuthErr(r *http.Request) error {
	val := r.Context().Value(basicAuthErrContextKey)
	if val == nil {
		return nil
	}

	return val.(error)
}

var DefaultBasicAuthErrorHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	err := GetBasicAuthErr(r)
	if errors.Is(err, ldap.ErrInvalidCredentials) {
		w.Header().Set("WWW-Authenticate", `Basic realm="restricted", charset="UTF-8"`)
		http.Error(w, err.Error(), http.StatusUnauthorized)
	} else {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
})

type BasicAuthData struct {
	Username string `json:"username"`
}

type BasicAuth struct {
	logger       *slog.Logger
	errorHandler http.Handler
	factory      *ldap.Factory
}

func NewBasicAuth(factory *ldap.Factory, errorHandler http.Handler, logger *slog.Logger) *BasicAuth {
	return &BasicAuth{
		logger:       logger,
		errorHandler: errorHandler,
		factory:      factory,
	}
}

func (ba *BasicAuth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		ctx, span := observability.TracerFromContext(ctx).Start(ctx, "BasicAuth.Middleware")
		defer span.End()

		username, password, ok := r.BasicAuth()
		if ok && len(username) > 0 {
			user, err := authenticate(ctx, ba.factory, username, password, ba.logger)
			if err != nil {
				r := r.WithContext(context.WithValue(ctx, basicAuthErrContextKey, err))
				ba.errorHandler.ServeHTTP(w, r)

				observability.SpanErr(span, err)

				return
			}

			auth := authFromUser(user, nil)
			r = r.WithContext(context.WithValue(ctx, authContextKey, &auth))
		}

		next.ServeHTTP(w, r)
	})
}

func (ba *BasicAuth) Required(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := GetAuth(r)
		if auth == nil {
			w.Header().Set("WWW-Authenticate", `Basic realm="restricted", charset="UTF-8"`)
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		} else {
			next.ServeHTTP(w, r)
		}
	})
}
