package kontakte

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/teapotovh/teapot/lib/httpauth"
	"github.com/teapotovh/teapot/lib/httplog"
	"github.com/teapotovh/teapot/lib/ldap"
	"github.com/teapotovh/teapot/lib/ui"
	"github.com/teapotovh/teapot/lib/webauth"
	"github.com/teapotovh/teapot/lib/webhandler"
)

// Kontakte is the kontakte HTTP server that renders web pages for users to
// modify their LDAP information.
type Kontakte struct {
	logger *slog.Logger

	httpLog    *httplog.HTTPLog
	webHandler *webhandler.WebHandler
	factory    *ldap.Factory
	webAuth    *webauth.WebAuth
}

type KontakteConfig struct {
	HTTPLog    httplog.HTTPLogConfig
	WebHandler webhandler.WebHandlerConfig
	LDAP       ldap.LDAPConfig
	WebAuth    webauth.WebAuthConfig
}

func NewKontakte(config KontakteConfig, logger *slog.Logger) (*Kontakte, error) {
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
		ReturnPath: func(auth httpauth.Auth) string {
			return PathUser(auth.Username)
		},
		App: App,
	}, logger.With("component", "auth"))
	if err != nil {
		return nil, fmt.Errorf("error while constructing webauth: %w", err)
	}

	kontakte := Kontakte{
		logger: logger,

		httpLog:    httpLog,
		webHandler: webHandler,
		factory:    factory,
		webAuth:    webAuth,
	}

	return &kontakte, nil
}

// Handler implements httpsrv.HTTPService.
func (k *Kontakte) Handler(prefix string) http.Handler {
	mux := http.NewServeMux()

	mux.Handle(k.webHandler.AssetPath, k.webHandler.AssetHandler)

	mux.Handle(PathLogin, k.webHandler.Adapt(k.webAuth.Login))
	mux.Handle(PathLogout, k.webHandler.Adapt(k.webAuth.Logout))

	mux.Handle(PathUsers, k.webHandler.Adapt(k.Users))
	mux.Handle(PathUser("{username}"), k.webHandler.Adapt(k.User))
	mux.Handle(PathPasswd("{username}"), k.webHandler.Adapt(k.Passwd))

	mux.Handle("/{path...}", k.webHandler.Adapt(k.NotFound))

	var handler http.Handler = mux

	handler = k.webAuth.Middleware(handler)
	handler = k.httpLog.LogMiddleware(handler)
	handler = k.httpLog.ExtractMiddleware(handler)

	return handler
}

func (k *Kontakte) NotFound(w http.ResponseWriter, r *http.Request) (ui.Component, error) {
	if r.URL.Path == PathIndex {
		return k.Index(w, r)
	}

	return nil, webhandler.ErrNotFound
}
