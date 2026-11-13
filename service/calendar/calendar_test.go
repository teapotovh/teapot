package calendar

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/teapotovh/teapot/lib/httpauth"
	"github.com/teapotovh/teapot/lib/httplog"
	"github.com/teapotovh/teapot/lib/ldap"
	"github.com/teapotovh/teapot/lib/wellknown"
	"github.com/teapotovh/teapot/service/calendar/backend"
	"github.com/teapotovh/teapot/service/calendar/store"
)

type fakeAuthenticator struct {
	users map[string]string
}

func (f *fakeAuthenticator) Authenticate(_ context.Context, username string, password string) (*ldap.User, error) {
	expected, ok := f.users[username]
	if !ok || expected != password {
		return nil, ldap.ErrInvalidCredentials
	}

	return &ldap.User{
		Username: username,
		Mail:     username + "@example.com",
	}, nil
}

func testLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func newTestCalendar(t *testing.T, users map[string]string) *Calendar {
	t.Helper()

	logger := testLogger()

	httpLog, err := httplog.NewHTTPLog(httplog.HTTPLogConfig{Level: "info"}, logger.With("component", "httplog"))
	require.NoError(t, err)

	store := store.NewMem()

	comps := calendarComponents{
		httpLog: httpLog,
		httpAuth: httpauth.NewBasicAuth(
			&fakeAuthenticator{users: users},
			httpauth.DefaultBasicAuthErrorHandler,
			logger.With("component", "auth"),
		),
		wellKnown: wellknown.WellKnown{
			wellknown.Caldav: wellknown.ResolverHost,
		},

		store:   store,
		backend: backend.NewBackend(store, logger.With("component", "backend")),
	}

	return newCalendarFromComponents(comps, logger)
}

func newTestServer(t *testing.T, users map[string]string) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(newTestCalendar(t, users).Handler("/"))
	t.Cleanup(srv.Close)

	return srv
}

func TestCalendarHandlerRequiresAuthentication(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, map[string]string{"testuser": "secret"})

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/", nil)
	require.NoError(t, err)

	resp, err := srv.Client().Do(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	require.NotEmpty(t, resp.Header.Get("WWW-Authenticate"))
}

func TestCalendarHandlerPropFindUserPrincipal(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, map[string]string{"testuser": "secret"})

	status, body, authenticate := propFindUserPrincipal(t, srv, "testuser", "secret")

	require.Equal(t, http.StatusMultiStatus, status)
	require.Empty(t, authenticate)
	require.Contains(t, body, "/testuser")
}

func TestCalendarHandlerPropFindWrongPassword(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, map[string]string{"testuser": "secret"})

	status, _, authenticate := propFindUserPrincipal(t, srv, "testuser", "wrong")

	require.Equal(t, http.StatusUnauthorized, status)
	require.NotEmpty(t, authenticate)
}

func propFindUserPrincipal(t *testing.T, srv *httptest.Server, username, password string) (int, string, string) {
	t.Helper()

	const body = `<D:propfind xmlns:D="DAV:"><D:prop><D:current-user-principal/></D:prop></D:propfind>`

	req, err := http.NewRequest("PROPFIND", srv.URL+"/testuser", strings.NewReader(body))
	require.NoError(t, err)

	req.Header.Set("Content-Type", "application/xml")
	req.Header.Set("Depth", "0")
	req.SetBasicAuth(username, password)

	resp, err := srv.Client().Do(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp.StatusCode, string(respBody), resp.Header.Get("WWW-Authenticate")
}
