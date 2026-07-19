package calendar

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/teapotovh/teapot/lib/httpauth"
	"github.com/teapotovh/teapot/lib/httplog"
	"github.com/teapotovh/teapot/lib/httptrace"
	"github.com/teapotovh/teapot/lib/ldap"
	"github.com/teapotovh/teapot/lib/webdav/caldav"
	"github.com/teapotovh/teapot/service/calendar/backend"
	"github.com/teapotovh/teapot/service/calendar/store"
	"go.opentelemetry.io/otel/trace"
)

type Calendar struct {
	logger *slog.Logger

	httpLog   *httplog.HTTPLog
	httpTrace *httptrace.HTTPTrace
	httpAuth  *httpauth.BasicAuth

	store   store.Store
	backend *backend.Backend
}

type CalendarConfig struct {
	HTTPLog httplog.HTTPLogConfig
	LDAP    ldap.LDAPConfig
	Store   store.StoreConfig
}

func NewCalendar(config CalendarConfig, logger *slog.Logger) (*Calendar, error) {
	// Provide request information in all log operations
	logger = httplog.WithHandler(logger)

	httpLog, err := httplog.NewHTTPLog(config.HTTPLog, logger.With("component", "httplog"))
	if err != nil {
		return nil, fmt.Errorf("error while constructing httplog: %w", err)
	}
	httpTrace := httptrace.NewHTTPTrace()

	ldapFactory, err := ldap.NewFactory(config.LDAP, logger.With("component", "ldap"))
	if err != nil {
		return nil, fmt.Errorf("error while building LDAP factory: %w", err)
	}

	httpAuth := httpauth.NewBasicAuth(
		ldapFactory,
		httpauth.DefaultBasicAuthErrorHandler,
		logger.With("component", "auth"),
	)

	store, err := store.NewStore(config.Store, logger.With("component", "store"))
	if err != nil {
		return nil, fmt.Errorf("error while initializing calendar store: %w", err)
	}

	backend := backend.NewBackend(store, logger.With("component", "backend"))

	calendar := Calendar{
		logger: logger,

		httpLog:   httpLog,
		httpTrace: httpTrace,
		httpAuth:  httpAuth,

		store:   store,
		backend: backend,
	}

	return &calendar, nil
}

func (c *Calendar) Store() store.Store {
	return c.store
}

func (c *Calendar) Handler(prefix string) http.Handler {
	var handler http.Handler = &caldav.Handler{
		Prefix:  prefix,
		Backend: c.backend,
	}

	handler = c.httpAuth.Required(handler)
	handler = c.httpAuth.Middleware(handler)
	handler = c.httpLog.LogMiddleware(handler)
	handler = c.httpLog.ExtractMiddleware(handler)
	handler = c.httpTrace.TracerMiddleware(handler)

	return handler
}

// WithTracing implements observability.Tracing.
func (c *Calendar) WithTracing(tp trace.TracerProvider, tracer trace.Tracer) {
	c.httpTrace.WithTracing(tp, tracer)
}
