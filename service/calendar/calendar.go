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

type calendarComponents struct {
	httpLog     *httplog.HTTPLog
	ldapFactory *ldap.Factory
	httpAuth    *httpauth.BasicAuth
	wellKnown   wellknown.WellKnown

	store   store.Store
	backend *backend.Backend
}

func newComponents(config CalendarConfig, logger *slog.Logger) (calendarComponents, error) {
	httpLog, err := httplog.NewHTTPLog(config.HTTPLog, logger.With("component", "httplog"))
	if err != nil {
		return calendarComponents{}, fmt.Errorf("error while constructing httplog: %w", err)
	}

	ldapFactory, err := ldap.NewFactory(config.LDAP, logger.With("component", "ldap"))
	if err != nil {
		return calendarComponents{}, fmt.Errorf("error while building LDAP factory: %w", err)
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
		return calendarComponents{}, fmt.Errorf("error while initializing calendar store: %w", err)
	}

	backend := backend.NewBackend(store, logger.With("component", "backend"))

	return calendarComponents{
		httpLog:     httpLog,
		ldapFactory: ldapFactory,
		httpAuth:    httpAuth,
		wellKnown:   wellKnown,

		store:   store,
		backend: backend,
	}, nil
}

func newCalendarFromComponents(comps calendarComponents, logger *slog.Logger) *Calendar {
	return &Calendar{
		logger: logger,

		httpLog:     comps.httpLog,
		ldapFactory: comps.ldapFactory,
		httpAuth:    comps.httpAuth,
		wellKnown:   comps.wellKnown,

		store:   comps.store,
		backend: comps.backend,
	}
}

func NewCalendar(config CalendarConfig, logger *slog.Logger) (*Calendar, error) {
	comps, err := newComponents(config, logger)
	if err != nil {
		return nil, err
	}

	return newCalendarFromComponents(comps, logger), nil
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
