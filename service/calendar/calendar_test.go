package calendar

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/teapotovh/teapot/lib/httpauth"
	"github.com/teapotovh/teapot/lib/httplog"
	"github.com/teapotovh/teapot/lib/ldap"
	"github.com/teapotovh/teapot/lib/webdav"
	"github.com/teapotovh/teapot/lib/wellknown"
	"github.com/teapotovh/teapot/service/calendar/backend"
	"github.com/teapotovh/teapot/service/calendar/store"

	_ "embed"
)

const (
	testUsername = "testuser"
	testPassword = "secret"

	principalPath   = "/testuser"
	calendarHomeSet = "/testuser/calendars/"
	calendarPath    = "/testuser/calendars/personal/"

	// The backend normalizes stored calendar paths by trimming the trailing
	// slash, so hrefs returned by PROPFIND use this form.
	calendarResourcePath = "/testuser/calendars/personal"
	eventPath            = calendarPath + "event.ics"
)

//go:embed testdata/calendar-query.xml
var calendarQueryXML string

//go:embed testdata/mkcalendar.xml
var mkcalendarXML string

//go:embed testdata/teapot-event.ics
var teapotEvent string

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

	httpLog, err := httplog.NewHTTPLog(httplog.HTTPLogConfig{
		Level:  "debug",
		Ignore: "(static|favicon)",
	}, logger.With("component", "httplog"))
	require.NoError(t, err)

	st := store.NewMem()

	return &Calendar{
		logger: logger,

		httpLog: httpLog,
		httpAuth: httpauth.NewBasicAuth(
			&fakeAuthenticator{users: users},
			httpauth.DefaultBasicAuthErrorHandler,
			logger.With("component", "auth"),
		),
		wellKnown: wellknown.WellKnown{
			wellknown.Caldav: wellknown.ResolverHost,
		},

		store:   st,
		backend: backend.NewBackend(st, logger.With("component", "backend")),
	}
}

func newTestServer(t *testing.T, users map[string]string) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(newTestCalendar(t, users).Handler("/"))
	t.Cleanup(srv.Close)

	return srv
}

type calendarRequest struct {
	body        string
	contentType string
	depth       string

	username string
	password string
}

func doCalendarRequest(
	t *testing.T,
	srv *httptest.Server,
	method string,
	path string,
	request calendarRequest,
) (int, http.Header, string) {
	t.Helper()

	var body io.Reader
	if request.body != "" {
		body = strings.NewReader(request.body)
	}

	req, err := http.NewRequest(method, srv.URL+path, body)
	require.NoError(t, err)

	if request.contentType != "" {
		req.Header.Set("Content-Type", request.contentType)
	}

	if request.depth != "" {
		req.Header.Set("Depth", request.depth)
	}

	if request.username != "" {
		req.SetBasicAuth(request.username, request.password)
	}

	resp, err := srv.Client().Do(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp.StatusCode, resp.Header, string(respBody)
}

func doAuthenticatedRequest(
	t *testing.T,
	srv *httptest.Server,
	method string,
	path string,
	request calendarRequest,
) (int, http.Header, string) {
	t.Helper()

	request.username = testUsername
	request.password = testPassword

	return doCalendarRequest(t, srv, method, path, request)
}

// multistatus only models the subset of the DAV:multistatus body used by the
// assertions below.
type multistatus struct {
	XMLName   xml.Name              `xml:"DAV: multistatus"`
	Responses []multistatusResponse `xml:"DAV: response"`
}

type multistatusResponse struct {
	Hrefs     []string              `xml:"DAV: href"`
	PropStats []multistatusPropstat `xml:"DAV: propstat"`
}

type multistatusPropstat struct {
	Prop multistatusProp `xml:"DAV: prop"`
}

type multistatusProp struct {
	CurrentUserPrincipal          *hrefProp                          `xml:"DAV: current-user-principal"`
	CalendarHomeSet               *hrefProp                          `xml:"urn:ietf:params:xml:ns:caldav calendar-home-set"`
	ResourceType                  *resourceTypeProp                  `xml:"DAV: resourcetype"`
	SupportedCalendarComponentSet *supportedCalendarComponentSetProp `xml:"urn:ietf:params:xml:ns:caldav supported-calendar-component-set"`
}

type supportedCalendarComponentSetProp struct {
	Comp []struct {
		Name string `xml:"name,attr"`
	} `xml:"urn:ietf:params:xml:ns:caldav comp"`
}

type hrefProp struct {
	Href string `xml:"DAV: href"`
}

type resourceTypeProp struct {
	Collection *struct{} `xml:"DAV: collection"`
	Calendar   *struct{} `xml:"urn:ietf:params:xml:ns:caldav calendar"`
}

func propFind(t *testing.T, srv *httptest.Server, path string, props string) (int, multistatus) {
	t.Helper()

	body := fmt.Sprintf(`<D:propfind xmlns:D="DAV:"><D:prop>%s</D:prop></D:propfind>`, props)

	status, _, respBody := doAuthenticatedRequest(t, srv, webdav.MethodPropFind, path, calendarRequest{
		body:        body,
		contentType: "application/xml",
		depth:       "0",
	})

	var ms multistatus
	if status == http.StatusMultiStatus {
		require.NoError(t, xml.Unmarshal([]byte(respBody), &ms))
	}

	return status, ms
}

func requireSingleResponse(t *testing.T, ms multistatus) multistatusResponse {
	t.Helper()

	require.Len(t, ms.Responses, 1)
	require.NotEmpty(t, ms.Responses[0].PropStats)

	return ms.Responses[0]
}

func TestCalendarHandlerRequiresAuthentication(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, map[string]string{testUsername: testPassword})

	status, headers, _ := doCalendarRequest(t, srv, http.MethodGet, "/", calendarRequest{})

	require.Equal(t, http.StatusUnauthorized, status)
	require.NotEmpty(t, headers.Get("WWW-Authenticate"))
}

func TestCalendarHandlerPropFindWrongPassword(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, map[string]string{testUsername: testPassword})

	status, headers, _ := doCalendarRequest(t, srv, webdav.MethodPropFind, principalPath, calendarRequest{
		body:        `<D:propfind xmlns:D="DAV:"><D:prop><D:current-user-principal/></D:prop></D:propfind>`,
		contentType: "application/xml",
		depth:       "0",
		username:    testUsername,
		password:    "wrong",
	})

	require.Equal(t, http.StatusUnauthorized, status)
	require.NotEmpty(t, headers.Get("WWW-Authenticate"))
}

func TestCalendarHandlerPropFindCurrentUserPrincipal(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, map[string]string{testUsername: testPassword})

	status, ms := propFind(t, srv, principalPath, `<D:current-user-principal/>`)
	require.Equal(t, http.StatusMultiStatus, status)

	resp := requireSingleResponse(t, ms)
	require.Equal(t, principalPath, resp.Hrefs[0])
	require.NotNil(t, resp.PropStats[0].Prop.CurrentUserPrincipal)
	require.Equal(t, principalPath, resp.PropStats[0].Prop.CurrentUserPrincipal.Href)
}

func TestCalendarHandlerPropFindCurrentUserPrincipalTrailingSlash(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, map[string]string{testUsername: testPassword})

	status, ms := propFind(t, srv, principalPath+"/", `<D:current-user-principal/>`)
	require.Equal(t, http.StatusMultiStatus, status)

	resp := requireSingleResponse(t, ms)
	require.Equal(t, principalPath, resp.Hrefs[0])
	require.NotNil(t, resp.PropStats[0].Prop.CurrentUserPrincipal)
	require.Equal(t, principalPath, resp.PropStats[0].Prop.CurrentUserPrincipal.Href)
}

func TestCalendarHandlerPropFindCalendarHomeSet(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, map[string]string{testUsername: testPassword})

	status, ms := propFind(t, srv, principalPath, `<C:calendar-home-set xmlns:C="urn:ietf:params:xml:ns:caldav"/>`)
	require.Equal(t, http.StatusMultiStatus, status)

	resp := requireSingleResponse(t, ms)
	require.NotNil(t, resp.PropStats[0].Prop.CalendarHomeSet)
	require.Equal(t, calendarHomeSet, resp.PropStats[0].Prop.CalendarHomeSet.Href)
}

func TestCalendarHandlerPropFindCalendarHomeSetCollection(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, map[string]string{testUsername: testPassword})

	status, ms := propFind(t, srv, calendarHomeSet, `<D:resourcetype/>`)
	require.Equal(t, http.StatusMultiStatus, status)

	resp := requireSingleResponse(t, ms)
	require.Equal(t, calendarHomeSet, resp.Hrefs[0])
	require.NotNil(t, resp.PropStats[0].Prop.ResourceType)
	require.NotNil(t, resp.PropStats[0].Prop.ResourceType.Collection)
	require.Nil(t, resp.PropStats[0].Prop.ResourceType.Calendar)
}

func TestCalendarHandlerPropFindCalendarHomeSetNoTrailingSlash(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, map[string]string{testUsername: testPassword})

	status, ms := propFind(t, srv, strings.TrimSuffix(calendarHomeSet, "/"), `<D:resourcetype/>`)
	require.Equal(t, http.StatusMultiStatus, status)

	resp := requireSingleResponse(t, ms)
	require.Equal(t, calendarHomeSet, resp.Hrefs[0])
	require.NotNil(t, resp.PropStats[0].Prop.ResourceType)
	require.NotNil(t, resp.PropStats[0].Prop.ResourceType.Collection)
}

func TestCalendarHandlerMKCalendar(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, map[string]string{testUsername: testPassword})

	status, _, _ := doAuthenticatedRequest(t, srv, webdav.MethodMkcalendar, calendarPath, calendarRequest{
		body:        mkcalendarXML,
		contentType: `application/xml; charset="utf-8"`,
	})
	require.Equal(t, http.StatusCreated, status)

	status, ms := propFind(t, srv, calendarPath, `<D:resourcetype/>`)
	require.Equal(t, http.StatusMultiStatus, status)

	resp := requireSingleResponse(t, ms)
	require.Equal(t, calendarResourcePath, resp.Hrefs[0])
	require.NotNil(t, resp.PropStats[0].Prop.ResourceType)
	require.NotNil(t, resp.PropStats[0].Prop.ResourceType.Collection)
	require.NotNil(t, resp.PropStats[0].Prop.ResourceType.Calendar)
}

func TestCalendarHandlerMKCalendarSupportedComponents(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, map[string]string{testUsername: testPassword})

	status, _, _ := doAuthenticatedRequest(t, srv, webdav.MethodMkcalendar, calendarPath, calendarRequest{
		body:        mkcalendarXML,
		contentType: `application/xml; charset="utf-8"`,
	})
	require.Equal(t, http.StatusCreated, status)

	status, ms := propFind(t, srv, calendarPath,
		`<C:supported-calendar-component-set xmlns:C="urn:ietf:params:xml:ns:caldav"/>`)
	require.Equal(t, http.StatusMultiStatus, status)

	resp := requireSingleResponse(t, ms)
	require.NotNil(t, resp.PropStats[0].Prop.SupportedCalendarComponentSet)

	var names []string

	for _, component := range resp.PropStats[0].Prop.SupportedCalendarComponentSet.Comp {
		names = append(names, component.Name)
	}

	assert.ElementsMatch(t, []string{"VEVENT", "VTODO"}, names)
}

func TestCalendarHandlerMKCalendarDuplicate(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, map[string]string{testUsername: testPassword})

	status, _, _ := doAuthenticatedRequest(t, srv, webdav.MethodMkcalendar, calendarPath, calendarRequest{
		body:        mkcalendarXML,
		contentType: `application/xml; charset="utf-8"`,
	})
	require.Equal(t, http.StatusCreated, status)

	status, _, _ = doAuthenticatedRequest(t, srv, webdav.MethodMkcalendar, calendarPath, calendarRequest{
		body:        mkcalendarXML,
		contentType: `application/xml; charset="utf-8"`,
	})

	// DAV:resource-must-be-null: the request URI must not map to an existing
	// resource, so a duplicate MKCALENDAR is rejected with 403 (RFC 4791 1.3).
	assert.Equal(t, http.StatusForbidden, status)
	assert.GreaterOrEqual(t, status, 400)
	assert.Less(t, status, 500)
}

func TestCalendarHandlerOptions(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, map[string]string{testUsername: testPassword})

	status, headers, _ := doAuthenticatedRequest(t, srv, http.MethodOptions, calendarPath, calendarRequest{})
	require.Equal(t, http.StatusNoContent, status)
	require.Contains(t, headers.Get("DAV"), "calendar-access")
}

func TestCalendarHandlerCalendarObjectLifecycle(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, map[string]string{testUsername: testPassword})

	status, _, _ := doAuthenticatedRequest(t, srv, webdav.MethodMkcalendar, calendarPath, calendarRequest{
		body:        mkcalendarXML,
		contentType: `application/xml; charset="utf-8"`,
	})
	require.Equal(t, http.StatusCreated, status)

	status, headers, _ := doAuthenticatedRequest(t, srv, http.MethodPut, eventPath, calendarRequest{
		body:        teapotEvent,
		contentType: "text/calendar; charset=utf-8",
	})
	require.Equal(t, http.StatusCreated, status)

	require.Empty(t, headers.Get("ETag"))

	status, headers, body := doAuthenticatedRequest(t, srv, http.MethodGet, eventPath, calendarRequest{})
	require.Equal(t, http.StatusOK, status)
	require.NotEmpty(t, headers.Get("ETag"))
	require.Contains(t, headers.Get("Content-Type"), "text/calendar")
	require.Contains(t, body, "BEGIN:VCALENDAR")
	require.Contains(t, body, "UID:event1")
	require.Contains(t, body, "SUMMARY:Test event")

	status, _, body = doAuthenticatedRequest(t, srv, webdav.MethodReport, calendarPath, calendarRequest{
		body:        calendarQueryXML,
		contentType: "application/xml",
		depth:       "1",
	})
	require.Equal(t, http.StatusMultiStatus, status)

	var ms multistatus
	require.NoError(t, xml.Unmarshal([]byte(body), &ms))
	require.Len(t, ms.Responses, 1)
	require.Equal(t, []string{eventPath}, ms.Responses[0].Hrefs)

	status, _, _ = doAuthenticatedRequest(t, srv, http.MethodDelete, eventPath, calendarRequest{})
	require.Equal(t, http.StatusNoContent, status)

	status, _, _ = doAuthenticatedRequest(t, srv, http.MethodGet, eventPath, calendarRequest{})
	require.Equal(t, http.StatusNotFound, status)
}

func TestCalendarHandlerWellKnownCaldav(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, map[string]string{testUsername: testPassword})

	client := *srv.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}

	expectedLocation := "//" + srv.Listener.Addr().String()

	for _, auth := range []bool{false, true} {
		req, err := http.NewRequest(http.MethodGet, srv.URL+wellknown.Prefix+string(wellknown.Caldav), nil)
		require.NoError(t, err)

		if auth {
			req.SetBasicAuth(testUsername, testPassword)
		}

		resp, err := client.Do(req)
		require.NoError(t, err)

		_ = resp.Body.Close()

		require.Equal(t, http.StatusPermanentRedirect, resp.StatusCode)
		require.Equal(t, expectedLocation, resp.Header.Get("Location"))
	}
}
