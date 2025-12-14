package httpauth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/teapotovh/teapot/lib/ldap"
)

const (
	basicAuthErrContextKey contextKey = "basic-auth-err"
)

type BasicAuthData struct {
	Username string `json:"username"`
}

type BasicAuth struct {
	logger       *slog.Logger
	errorHandler http.Handler
	factory      *ldap.Factory
}

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

func NewBasicAuth(factory *ldap.Factory, errorHandler http.Handler, logger *slog.Logger) *BasicAuth {
	return &BasicAuth{
		logger:       logger,
		errorHandler: errorHandler,
		factory:      factory,
	}
}

func (ba *BasicAuth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if ok {
			client, err := ba.factory.NewClient(r.Context())
			if err != nil {
				ba.logger.Error("error while creating LDAP client", "err", err)
				ba.errorHandler.ServeHTTP(w, r)
				return
			}
			defer client.Close()

			user, err := client.Authenticate(username, password)
			if err != nil {
				if !errors.Is(err, ldap.ErrInvalidCredentials) {
					ba.logger.Error("error while authenticating", "username", username, "err", err)
					err = ldap.ErrInvalidCredentials
				}

				r := r.WithContext(context.WithValue(r.Context(), basicAuthErrContextKey, err))
				ba.errorHandler.ServeHTTP(w, r)
				return
			}

			auth := authFromUser(user, nil)
			r = r.WithContext(context.WithValue(r.Context(), authContextKey, &auth))
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
