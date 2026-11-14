package caldavtest

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/teapotovh/teapot/lib/webdav"
)

const supportedReportSetQuery = `<D:propfind xmlns:D="DAV:"><D:prop><D:supported-report-set/></D:prop></D:propfind>`

// RFC 4791, section 5.1: https://www.rfc-editor.org/rfc/rfc4791#section-5.1
func TestOptionsDAVHeader(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)

	res := doRequest(t, srv, http.MethodOptions, calendarPath(userOne))
	require.GreaterOrEqual(t, res.Status, 200)
	require.Less(t, res.Status, 300)

	tokens := headerTokens(res.Header.Get("DAV"))
	assert.ElementsMatch(t, []string{"1", "3", "calendar-access"}, tokens)

	for _, capability := range []string{
		"access-control",
		"calendar-schedule",
		"calendar-auto-schedule",
		"sync-collection",
	} {
		assert.NotContains(t, tokens, capability)
	}
}

// RFC 9110, section 10.2.1: https://www.rfc-editor.org/rfc/rfc9110#section-10.2.1
func TestOptionsAllowHeader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		path      string
		createObj bool
		wantAllow []string
	}{
		{
			name:      "calendar collection",
			path:      calendarPath(userOne),
			wantAllow: []string{http.MethodOptions, webdav.MethodPropFind, webdav.MethodReport},
		},
		{
			name:      "calendar home",
			path:      calendarHome(userOne),
			wantAllow: []string{http.MethodOptions, webdav.MethodPropFind, webdav.MethodMkcalendar},
		},
		{
			name:      "principal",
			path:      principal(userOne),
			wantAllow: []string{http.MethodOptions, webdav.MethodPropFind},
		},
		{
			name:      "existing calendar object",
			path:      calendarPath(userOne) + "event.ics",
			createObj: true,
			wantAllow: []string{
				http.MethodOptions,
				http.MethodHead,
				http.MethodGet,
				http.MethodPut,
				http.MethodDelete,
				webdav.MethodPropFind,
			},
		},
		{
			name:      "missing calendar object",
			path:      calendarPath(userOne) + "missing.ics",
			wantAllow: []string{http.MethodOptions, http.MethodPut},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := newTestServer(t)

			if tt.createObj {
				body := loadAppleFixture(t, "Resource/CalDAV/delete/1.txt", nil)
				res := putObject(t, srv, userOne, tt.path, body)
				require.Equal(t, http.StatusCreated, res.Status, "body: %s", res.Body)
			}

			res := doRequest(t, srv, http.MethodOptions, tt.path)
			require.GreaterOrEqual(t, res.Status, 200)
			require.Less(t, res.Status, 300)

			assert.ElementsMatch(t, tt.wantAllow, headerTokens(res.Header.Get("Allow")))
		})
	}
}

// RFC 4791, section 2: https://www.rfc-editor.org/rfc/rfc4791#section-2
func TestSupportedReportSet(t *testing.T) {
	t.Parallel()

	t.Run("calendar collection", func(t *testing.T) {
		t.Parallel()

		srv := newTestServer(t)

		res := propFind(t, srv, calendarPath(userOne), withDepth("0"), withBody(supportedReportSetQuery))
		require.Equal(t, http.StatusMultiStatus, res.Status, "body: %s", res.Body)

		ms := parseMultiStatus(t, res.Body)

		response, ok := ms.findResponse(calendarPath(userOne))
		require.True(t, ok, "missing response for calendar collection")

		propstat, ok := response.findPropstat(t, http.StatusOK)
		require.True(t, ok, "missing 200 propstat")
		require.NotNil(t, propstat.Prop.SupportedReportSet)

		assertReports(t, propstat.Prop.SupportedReportSet)
	})

	t.Run("calendar object", func(t *testing.T) {
		t.Parallel()

		srv := newTestServer(t)
		objectPath := calendarPath(userOne) + "event.ics"

		body := loadAppleFixture(t, "Resource/CalDAV/delete/1.txt", nil)
		res := putObject(t, srv, userOne, objectPath, body)
		require.Equal(t, http.StatusCreated, res.Status, "body: %s", res.Body)

		res = propFind(t, srv, objectPath, withDepth("0"), withBody(supportedReportSetQuery))
		require.Equal(t, http.StatusMultiStatus, res.Status, "body: %s", res.Body)

		ms := parseMultiStatus(t, res.Body)

		response, ok := ms.findResponse(objectPath)
		require.True(t, ok, "missing response for calendar object")

		propstat, ok := response.findPropstat(t, http.StatusOK)
		require.True(t, ok, "missing 200 propstat")
		require.NotNil(t, propstat.Prop.SupportedReportSet)

		assertReports(t, propstat.Prop.SupportedReportSet)
	})
}

func assertReports(t *testing.T, set *supportedReportSet) {
	t.Helper()

	names := set.names()
	locals := make([]string, 0, len(names))

	for _, name := range names {
		assert.Equal(t, caldavNamespace, name.Space)
		locals = append(locals, name.Local)
	}

	assert.ElementsMatch(t, []string{"calendar-query", "calendar-multiget"}, locals)

	for _, name := range []string{
		"free-busy-query",
		"sync-collection",
		"expand-property",
	} {
		assert.NotContains(t, locals, name)
	}
}

// RFC 4791, section 7.10: https://www.rfc-editor.org/rfc/rfc4791#section-7.10
func TestUnsupportedReport(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)

	body := `<C:free-busy-query xmlns:C="urn:ietf:params:xml:ns:caldav">` +
		`<C:time-range start="20060104T000000Z" end="20060105T000000Z"/>` +
		`</C:free-busy-query>`

	res := report(t, srv, calendarPath(userOne), body)
	require.Equal(t, http.StatusForbidden, res.Status, "body: %s", res.Body)

	davErr := parseDAVError(t, res.Body)
	assert.True(t, hasName(davErr.Names, davNamespace, "supported-report"), "body: %s", res.Body)
}

// RFC 4791, section 5.3.2.1: https://www.rfc-editor.org/rfc/rfc4791#section-5.3.2.1
func TestPutUnsupportedContentType(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)

	body := loadAppleFixture(t, "Resource/CalDAV/delete/1.txt", nil)
	res := doRequest(
		t,
		srv,
		http.MethodPut,
		calendarPath(userOne)+"event.ics",
		withContentType("text/plain"),
		withBody(body),
	)
	require.Equal(t, http.StatusForbidden, res.Status, "body: %s", res.Body)

	davErr := parseDAVError(t, res.Body)
	assert.True(
		t,
		hasName(davErr.Names, caldavNamespace, "supported-calendar-data"),
		"body: %s",
		res.Body,
	)
}

// RFC 4791, section 5.3.2.1: https://www.rfc-editor.org/rfc/rfc4791#section-5.3.2.1
func TestPutMalformedCalendarData(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)

	res := putObject(t, srv, userOne, calendarPath(userOne)+"event.ics", "this is not a calendar")
	require.Equal(t, http.StatusForbidden, res.Status, "body: %s", res.Body)

	davErr := parseDAVError(t, res.Body)
	assert.True(t, hasName(davErr.Names, caldavNamespace, "valid-calendar-data"), "body: %s", res.Body)
}

// RFC 9110, section 15.5.6: https://www.rfc-editor.org/rfc/rfc9110#section-15.5.6
func TestUnsupportedMethod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		method string
	}{
		{
			name:   "acl",
			method: webdav.MethodACL,
		},
		{
			name:   "post",
			method: http.MethodPost,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := newTestServer(t)

			res := doRequest(t, srv, tt.method, calendarPath(userOne))
			assert.Equal(t, http.StatusMethodNotAllowed, res.Status, "body: %s", res.Body)
			assert.NotEmpty(t, res.Header.Get("Allow"))
		})
	}
}
