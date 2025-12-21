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

type WebAuthOptions struct {
	LoginPath  string
	LogoutPath string
	ReturnPath string
	App        string
}

type WebAuth struct {
	logger *slog.Logger

	auth     *httpauth.JWTAuth
	resetURL *url.URL

	loginPath  string
	logoutPath string
	returnPath string
	app        string
}

func NewWebAuth(
	factory *ldap.Factory,
	config WebAuthConfig,
	options WebAuthOptions,
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

		loginPath:  options.LoginPath,
		logoutPath: options.LogoutPath,
		returnPath: options.ReturnPath,
		app:        options.App,
	}

	return &wa, nil
}

func (wa *WebAuth) Middleware(next http.Handler) http.Handler {
	return wa.auth.Middleware(next)
}

var GetAuth = httpauth.GetAuth

type Auth = httpauth.Auth
