package calendar

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/teapotovh/teapot/lib/httpauth"
	"github.com/teapotovh/teapot/lib/httplog"
	"github.com/teapotovh/teapot/lib/ldap"
	"github.com/teapotovh/teapot/lib/webdav/caldav"
	"github.com/teapotovh/teapot/lib/wellknown"
	"github.com/teapotovh/teapot/service/calendar/backend"
	"github.com/teapotovh/teapot/service/calendar/store"
)

type Calendar struct {
	logger *slog.Logger

	httpLog     *httplog.HTTPLog
	ldapFactory *ldap.Factory
	httpAuth    *httpauth.BasicAuth
	wellKnown   wellknown.WellKnown

	store   store.Store
	backend *backend.Backend
}

type CalendarConfig struct {
	HTTPLog httplog.HTTPLogConfig
	LDAP    ldap.LDAPConfig
	Store   store.StoreConfig
}

func NewCalendar(config CalendarConfig, logger *slog.Logger) (*Calendar, error) {
	httpLog, err := httplog.NewHTTPLog(config.HTTPLog, logger.With("component", "httplog"))
	if err != nil {
		return nil, fmt.Errorf("error while constructing httplog: %w", err)
	}

	ldapFactory, err := ldap.NewFactory(config.LDAP, logger.With("component", "ldap"))
	if err != nil {
		return nil, fmt.Errorf("error while building LDAP factory: %w", err)
	}

	httpAuth := httpauth.NewBasicAuth(
		ldapFactory,
		httpauth.DefaultBasicAuthErrorHandler,
		logger.With("component", "auth"),
	)

	wellKnown := wellknown.WellKnown{
		wellknown.Caldav: wellknown.ResolverHost,
	}

	store, err := store.NewStore(config.Store, logger.With("component", "store"))
	if err != nil {
		return nil, fmt.Errorf("error while initializing calendar store: %w", err)
	}

	backend := backend.NewBackend(store, logger.With("component", "backend"))

	calendar := Calendar{
		logger: logger,

		httpLog:     httpLog,
		ldapFactory: ldapFactory,
		httpAuth:    httpAuth,
		wellKnown:   wellKnown,

		store:   store,
		backend: backend,
	}

	return &calendar, nil
}

func (c *Calendar) Store() store.Store {
	return c.store
}

func (c *Calendar) LDAPFactory() *ldap.Factory {
	return c.ldapFactory
}

func (c *Calendar) Handler(prefix string) http.Handler {
	mux := http.NewServeMux()

	var caldav http.Handler = &caldav.Handler{
		Prefix:  prefix,
		Backend: c.backend,
	}

	caldav = c.httpAuth.Required(caldav)
	caldav = c.httpAuth.Middleware(caldav)

	mux.Handle(wellknown.Prefix, c.wellKnown)
	mux.Handle("/{path...}", caldav)

	handler := c.httpLog.LogMiddleware(mux)
	handler = c.httpLog.ExtractMiddleware(handler)

	return handler
}
