package webauth

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/teapotovh/teapot/lib/httpauth"
	"github.com/teapotovh/teapot/lib/ldap"
)

type WebAuthConfig struct {
	JWTAuth          httpauth.JWTAuthConfig
	ResetPasswordURL string
}

type WebAuthPaths struct {
	Login  string
	Logout string
	Return string
}

type WebAuth struct {
	logger *slog.Logger

	auth     *httpauth.JWTAuth
	resetURL *url.URL

	loginPath  string
	logoutPath string
	returnPath string
}

func NewWebAuth(
	factory *ldap.Factory,
	config WebAuthConfig,
	paths WebAuthPaths,
	logger *slog.Logger,
) (*WebAuth, error) {
	auth := httpauth.NewJWTAuth(factory, config.JWTAuth, logger)

	resetURL, err := url.Parse(config.ResetPasswordURL)
	if err != nil {
		return nil, fmt.Errorf("error while parsing reset password URL %q: %w", config.ResetPasswordURL, err)
	}

	wa := WebAuth{
		logger: logger,

		auth:     auth,
		resetURL: resetURL,

		loginPath:  paths.Login,
		logoutPath: paths.Logout,
		returnPath: paths.Return,
	}

	return &wa, nil
}

func (wa *WebAuth) Middleware(next http.Handler) http.Handler {
	return wa.auth.Middleware(next)
}

var GetAuth = httpauth.GetAuth

type Auth = httpauth.Auth
