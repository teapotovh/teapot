package auth

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/zitadel/oidc/v3/pkg/op"

	"github.com/teapotovh/teapot/lib/httplog"
	"github.com/teapotovh/teapot/lib/ldap"
	"github.com/teapotovh/teapot/lib/webauth"
	"github.com/teapotovh/teapot/lib/webhandler"
)

type AuthConfig struct {
	HTTPLog    httplog.HTTPLogConfig
	WebHandler webhandler.WebHandlerConfig
	LDAP       ldap.LDAPConfig
	WebAuth    webauth.WebAuthConfig

	Key      []byte
	Duration time.Duration
}

type Auth struct {
	logger *slog.Logger

	httpLog    *httplog.HTTPLog
	webHandler *webhandler.WebHandler
	webAuth    *webauth.WebAuth

	oidc *op.Provider
}

func NewAuth(config AuthConfig, logger *slog.Logger) (*Auth, error) {
	// Provide request information in all log operations
	logger = httplog.WithHandler(logger)

	httpLog, err := httplog.NewHTTPLog(config.HTTPLog, logger.With("component", "httplog"))
	if err != nil {
		return nil, fmt.Errorf("error while constructing httplog: %w", err)
	}

	webHandler, err := webhandler.NewWebHandler(
		config.WebHandler,
		Skeleton,
		webhandler.DefaultErrorHandlers,
		logger.With("component", "webhandler"),
	)
	if err != nil {
		return nil, fmt.Errorf("error while constructing webhandler: %w", err)
	}

	factory, err := ldap.NewFactory(config.LDAP, logger.With("component", "ldap"))
	if err != nil {
		return nil, fmt.Errorf("error while building LDAP factory: %w", err)
	}

	webAuth, err := webauth.NewWebAuth(factory, config.WebAuth, webauth.WebAuthOptions{
		LoginPath:  PathLogin,
		LogoutPath: PathLogout,
		ReturnPath: webauth.ConstantPath(PathRedirect),
		App:        App,
	}, logger.With("component", "auth"))
	if err != nil {
		return nil, fmt.Errorf("error while constructing webauth: %w", err)
	}

	oidc, err := newOIDCProvider(oidcConfig{key: config.Key, duration: config.Duration}, logger.With("component", "oidc"))
	if err != nil {
		return nil, fmt.Errorf("error while constructing OpenID Connect provider: %w", err)
	}

	auth := Auth{
		logger: logger,

		httpLog:    httpLog,
		webHandler: webHandler,
		webAuth:    webAuth,

		oidc: oidc,
	}

	return &auth, nil
}

// Handler implements httpsrv.HTTPService.
func (k *Auth) Handler(prefix string) http.Handler {
	mux := http.NewServeMux()

	mux.Handle(k.webHandler.AssetPath, k.webHandler.AssetHandler)

	mux.Handle(PathLogin, k.webHandler.Adapt(k.webAuth.Login))
	mux.Handle(PathLogout, k.webHandler.Adapt(k.webAuth.Logout))

	mux.Handle(PathRedirect, k.webHandler.Adapt(k.Redirect))

	var handler http.Handler = mux

	handler = k.webAuth.Middleware(handler)
	handler = k.httpLog.LogMiddleware(handler)
	handler = k.httpLog.ExtractMiddleware(handler)

	return handler
}
