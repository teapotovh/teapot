package caldav_test

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/teapotovh/teapot/lib/httpauth"
	"github.com/teapotovh/teapot/lib/webdav/caldav"

	_ "embed"
)

const (
	principalPath = "/user/"
	homeSetPath   = "/user/calendars/"
	calendarPath  = "/user/calendars/test/"
	objectPath    = calendarPath + "event1.ics"

	contentTypeCalendar = "text/calendar"
	contentTypeXML      = "application/xml; charset=utf-8"

	headerContentType = "Content-Type"
	headerDate        = "Last-Modified"
	headerDepth       = "Depth"
	headerETag        = "ETag"
)

var errInvalidHTTPStatus = errors.New("invalid HTTP status")

//go:embed testdata/mkcalendar_rfc_5_3_1_2.xml
var mkcalendarRFC5312 string

//go:embed testdata/propfind_root.xml
var propfindRoot string

//go:embed testdata/propfind_principal.xml
var propfindPrincipal string

//go:embed testdata/propfind_home_set.xml
var propfindHomeSet string

//go:embed testdata/propfind_calendar.xml
var propfindCalendar string

//go:embed testdata/propfind_calendar_object.xml
var propfindCalendarObject string

//go:embed testdata/propfind_unknown_property.xml
var propfindUnknownProperty string

//go:embed testdata/proppatch_propertyupdate.xml
var proppatchPropertyUpdate string

//go:embed testdata/report_calendar_query_uid.xml
var reportCalendarQueryUID string

//go:embed testdata/report_calendar_query_events_only.xml
var reportCalendarQueryEventsOnly string

//go:embed testdata/report_calendar_multiget.xml
var reportCalendarMultiget string

//go:embed testdata/report_free_busy.xml
var reportFreeBusy string

//go:embed testdata/rfc4791-event-single.ics
var rfc4791EventSingle string

//go:embed testdata/rfc4791-event-recurring.ics
var rfc4791EventRecurring string

//go:embed testdata/rfc4791-event-attendees.ics
var rfc4791EventAttendees string

//go:embed testdata/rfc4791-todo-due-jan-4.ics
var rfc4791TodoDueJan4 string

//go:embed testdata/rfc4791-todo-due-jan-6.ics
var rfc4791TodoDueJan6 string

//go:embed testdata/rfc4791-todo-completed.ics
var rfc4791TodoCompleted string

//go:embed testdata/rfc4791-todo-cancelled.ics
var rfc4791TodoCancelled string

//go:embed testdata/rfc4791-freebusy.ics
var rfc4791Freebusy string

//go:embed testdata/teapot-event1.ics
var teapotEvent1 string

//go:embed testdata/teapot-event1_updated.ics
var teapotEvent1Updated string

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	srv, _ := newTestServerWithBackend(t)

	return srv
}

func newTestServerWithBackend(t *testing.T) (*httptest.Server, *fakeBackend) {
	t.Helper()

	b := newFakeBackend()
	h := &caldav.Handler{
		Prefix:  "/",
		Backend: b,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := httpauth.NewAuthContext(r.Context(), &httpauth.Auth{Username: "user"})
		h.ServeHTTP(w, r.WithContext(ctx))
	}))
	t.Cleanup(srv.Close)

	return srv, b
}

type httpResult struct {
	Status int
	Header http.Header
	Body   string
}

func doRequest(t *testing.T, srv *httptest.Server, method, reqPath, body string, headers map[string]string) httpResult {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}

	req, err := http.NewRequestWithContext(t.Context(), method, srv.URL+reqPath, reader)
	require.NoError(t, err)

	for name, value := range headers {
		req.Header.Set(name, value)
	}

	resp, err := srv.Client().Do(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return httpResult{
		Status: resp.StatusCode,
		Header: resp.Header,
		Body:   string(raw),
	}
}

func createCalendar(t *testing.T, srv *httptest.Server) {
	t.Helper()

	res := doRequest(t, srv, "MKCALENDAR", calendarPath, "", nil)
	require.Equal(t, http.StatusCreated, res.Status, "body: %s", res.Body)
}

func putCalendarObject(t *testing.T, srv *httptest.Server, reqPath, data string) httpResult {
	t.Helper()

	return doRequest(t, srv, http.MethodPut, reqPath, data, map[string]string{
		headerContentType: contentTypeCalendar,
	})
}

func propFind(t *testing.T, srv *httptest.Server, reqPath, depth, body string) httpResult {
	t.Helper()

	return doRequest(t, srv, "PROPFIND", reqPath, body, map[string]string{
		headerContentType: contentTypeXML,
		headerDepth:       depth,
	})
}

func report(t *testing.T, srv *httptest.Server, reqPath, body string) httpResult {
	t.Helper()

	return doRequest(t, srv, "REPORT", reqPath, body, map[string]string{
		headerContentType: contentTypeXML,
		headerDepth:       "1",
	})
}

type httpStatus struct {
	Code int
}

func (s *httpStatus) UnmarshalText(b []byte) error {
	fields := strings.Fields(string(b))
	if len(fields) < 2 {
		return fmt.Errorf("%w: %q", errInvalidHTTPStatus, string(b))
	}

	code, err := strconv.Atoi(fields[1])
	if err != nil {
		return fmt.Errorf("%w: %q: %w", errInvalidHTTPStatus, string(b), err)
	}

	s.Code = code

	return nil
}

type multiStatusProp struct {
	DisplayName         *string `xml:"displayname"`
	Description         *string `xml:"calendar-description"`
	GetETag             *string `xml:"getetag"`
	GetContentType      *string `xml:"getcontenttype"`
	GetContentLength    *int64  `xml:"getcontentlength"`
	GetLastModified     *string `xml:"getlastmodified"`
	CalendarData        *string `xml:"calendar-data"`
	CalendarHomeSetHref *string `xml:"calendar-home-set>href"`
	CurrentPrincipal    *string `xml:"current-user-principal>href"`

	SupportedComponents []struct {
		Name string `xml:"name,attr"`
	} `xml:"supported-calendar-component-set>comp"`

	ResourceType struct {
		Collection *struct{} `xml:"collection"`
		Calendar   *struct{} `xml:"calendar"`
		Principal  *struct{} `xml:"principal"`
	} `xml:"resourcetype"`
}

type multiStatusPropStat struct {
	Prop   multiStatusProp `xml:"prop"`
	Status httpStatus      `xml:"status"`
}

type multiStatusResponse struct {
	Href      string                `xml:"href"`
	Status    httpStatus            `xml:"status"`
	PropStats []multiStatusPropStat `xml:"propstat"`
}

type multiStatus struct {
	Responses []multiStatusResponse `xml:"response"`
}

func parseMultiStatus(t *testing.T, body string) multiStatus {
	t.Helper()

	var ms multiStatus
	require.NoError(t, xml.Unmarshal([]byte(body), &ms), "body: %s", body)

	return ms
}

func (r multiStatusResponse) propStat(code int) (multiStatusPropStat, bool) {
	for _, ps := range r.PropStats {
		if ps.Status.Code == code {
			return ps, true
		}
	}

	return multiStatusPropStat{}, false
}

func (r multiStatusResponse) requirePropStat(t *testing.T, code int) multiStatusPropStat {
	t.Helper()

	ps, ok := r.propStat(code)
	require.True(t, ok, "missing propstat with status %d", code)

	return ps
}

func hrefs(ms multiStatus) []string {
	out := make([]string, 0, len(ms.Responses))
	for _, resp := range ms.Responses {
		out = append(out, path.Clean(resp.Href))
	}

	sort.Strings(out)

	return out
}

func TestOptions(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)

	res := doRequest(t, srv, http.MethodOptions, homeSetPath, "", nil)
	require.Equal(t, http.StatusNoContent, res.Status)

	dav := res.Header.Get("DAV")
	assert.Contains(t, dav, "1")
	assert.Contains(t, dav, "3")
	assert.Contains(t, dav, "calendar-access")

	allow := res.Header.Get("Allow")
	for _, method := range []string{"OPTIONS", "PROPFIND", "REPORT", "DELETE", "MKCOL", "MKCALENDAR"} {
		assert.Contains(t, allow, method)
	}
}

func TestMkcalendar(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)

	res := doRequest(t, srv, "MKCALENDAR", calendarPath, mkcalendarRFC5312, map[string]string{
		headerContentType: contentTypeXML,
	})
	require.Equal(t, http.StatusCreated, res.Status, "body: %s", res.Body)

	res = propFind(t, srv, calendarPath, "0", propfindCalendar)
	require.Equal(t, http.StatusMultiStatus, res.Status, "body: %s", res.Body)

	ms := parseMultiStatus(t, res.Body)
	require.Len(t, ms.Responses, 1)
	require.Equal(t, path.Clean(calendarPath), path.Clean(ms.Responses[0].Href))

	ps := ms.Responses[0].requirePropStat(t, http.StatusOK)
	assert.NotNil(t, ps.Prop.ResourceType.Collection)
	assert.NotNil(t, ps.Prop.ResourceType.Calendar)
	require.NotNil(t, ps.Prop.DisplayName)
	assert.Equal(t, "Lisa's Events", *ps.Prop.DisplayName)
	require.NotNil(t, ps.Prop.Description)
	assert.Equal(t, "Calendar restricted to events.", *ps.Prop.Description)

	_, ok := ms.Responses[0].propStat(http.StatusNotFound)
	assert.True(t, ok, "calendar-timezone and getetag are not supported and should yield a 404 propstat")
}

func TestMkcol(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)

	res := doRequest(t, srv, "MKCOL", "/user/calendars/plain/", "", nil)
	require.Equal(t, http.StatusCreated, res.Status, "body: %s", res.Body)
}

func TestCalendarObjectLifecycle(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	createCalendar(t, srv)

	res := putCalendarObject(t, srv, objectPath, teapotEvent1)
	require.Equal(t, http.StatusCreated, res.Status, "body: %s", res.Body)

	etag := res.Header.Get(headerETag)
	require.NotEmpty(t, etag)
	assert.NotEmpty(t, res.Header.Get(headerDate))
	assert.Equal(t, objectPath, res.Header.Get("Location"))

	res = doRequest(t, srv, http.MethodGet, objectPath, "", nil)
	require.Equal(t, http.StatusOK, res.Status, "body: %s", res.Body)
	assert.Equal(t, contentTypeCalendar, res.Header.Get(headerContentType))
	assert.Equal(t, etag, res.Header.Get(headerETag))
	assert.Contains(t, res.Body, "BEGIN:VCALENDAR")
	assert.Contains(t, res.Body, "SUMMARY:Put Event")
	assert.Contains(t, res.Body, "UID:put-event@example.com")

	res = doRequest(t, srv, http.MethodHead, objectPath, "", nil)
	require.Equal(t, http.StatusOK, res.Status)
	assert.Empty(t, res.Body)
	assert.Equal(t, etag, res.Header.Get(headerETag))
	assert.Equal(t, contentTypeCalendar, res.Header.Get(headerContentType))
	assert.NotEmpty(t, res.Header.Get("Content-Length"))

	res = doRequest(t, srv, http.MethodDelete, objectPath, "", nil)
	require.Equal(t, http.StatusNoContent, res.Status, "body: %s", res.Body)
}

func TestUpdateCalendarObject(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	createCalendar(t, srv)

	created := putCalendarObject(t, srv, objectPath, teapotEvent1)
	require.Equal(t, http.StatusCreated, created.Status, "body: %s", created.Body)

	updated := putCalendarObject(t, srv, objectPath, teapotEvent1Updated)
	require.Equal(t, http.StatusNoContent, updated.Status, "body: %s", updated.Body)
	assert.Empty(t, updated.Header.Get("Location"))
	assert.NotEqual(t, created.Header.Get(headerETag), updated.Header.Get(headerETag))

	res := doRequest(t, srv, http.MethodGet, objectPath, "", nil)
	require.Equal(t, http.StatusOK, res.Status, "body: %s", res.Body)
	assert.Equal(t, updated.Header.Get(headerETag), res.Header.Get(headerETag))
	assert.Contains(t, res.Body, "SUMMARY:Put Event Updated")
}

func TestPutCalendarObjectOmitsETagForModifiedData(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	createCalendar(t, srv)

	// The server normalizes line endings, so the stored bytes differ from the
	// submitted bytes.
	body := strings.ReplaceAll(teapotEvent1, "\n", "\r\n")

	res := putCalendarObject(t, srv, objectPath, body)
	require.Equal(t, http.StatusCreated, res.Status, "body: %s", res.Body)
	assert.Empty(t, res.Header.Get(headerETag))

	get := doRequest(t, srv, http.MethodGet, objectPath, "", nil)
	require.Equal(t, http.StatusOK, get.Status, "body: %s", get.Body)
	assert.NotEmpty(t, get.Header.Get(headerETag))
	assert.Contains(t, get.Body, "SUMMARY:Put Event")
}

func TestPutCalendarObjectRejectsInvalidRequests(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	createCalendar(t, srv)

	tests := []struct {
		name        string
		contentType string
		body        string
		want        int
		wantBody    string
	}{
		{
			name:        "wrong content type",
			contentType: "text/plain",
			body:        teapotEvent1,
			want:        http.StatusBadRequest,
		},
		{
			name:        "malformed calendar data",
			contentType: contentTypeCalendar,
			body:        "this is not a calendar",
			want:        http.StatusBadRequest,
		},
		{
			name:        "empty body",
			contentType: contentTypeCalendar,
			body:        "",
			want:        http.StatusConflict,
			wantBody:    "valid-calendar-data",
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			reqPath := fmt.Sprintf("%sinvalid%d.ics", calendarPath, i)

			res := doRequest(t, srv, http.MethodPut, reqPath, tt.body, map[string]string{
				headerContentType: tt.contentType,
			})
			assert.Equal(t, tt.want, res.Status, "body: %s", res.Body)

			if tt.wantBody != "" {
				assert.Contains(t, res.Body, tt.wantBody)
			}
		})
	}
}

func TestPutCalendarObjectConditional(t *testing.T) {
	t.Parallel()

	srv, b := newTestServerWithBackend(t)
	createCalendar(t, srv)

	res := doRequest(t, srv, http.MethodPut, objectPath, teapotEvent1Updated, map[string]string{
		headerContentType: contentTypeCalendar,
		"If-Match":        `"current"`,
		"If-None-Match":   `"other"`,
	})
	require.Equal(t, http.StatusCreated, res.Status, "body: %s", res.Body)

	opts, ok := b.lastPutOptions()
	require.True(t, ok, "PutCalendarObject was not called")
	assert.Equal(t, `"current"`, string(opts.IfMatch))
	assert.Equal(t, `"other"`, string(opts.IfNoneMatch))
}

func TestPropFindRoot(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)

	res := propFind(t, srv, "/", "0", propfindRoot)
	require.Equal(t, http.StatusMultiStatus, res.Status, "body: %s", res.Body)

	ms := parseMultiStatus(t, res.Body)
	require.Len(t, ms.Responses, 1)

	ps := ms.Responses[0].requirePropStat(t, http.StatusOK)
	assert.NotNil(t, ps.Prop.ResourceType.Collection)
	require.NotNil(t, ps.Prop.CurrentPrincipal)
	assert.Equal(t, principalPath, *ps.Prop.CurrentPrincipal)
}

func TestPropFindPrincipal(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)

	res := propFind(t, srv, principalPath, "0", propfindPrincipal)
	require.Equal(t, http.StatusMultiStatus, res.Status, "body: %s", res.Body)

	ms := parseMultiStatus(t, res.Body)
	require.Len(t, ms.Responses, 1)

	ps := ms.Responses[0].requirePropStat(t, http.StatusOK)
	assert.NotNil(t, ps.Prop.ResourceType.Collection)
	assert.NotNil(t, ps.Prop.ResourceType.Principal)
	require.NotNil(t, ps.Prop.CurrentPrincipal)
	assert.Equal(t, principalPath, *ps.Prop.CurrentPrincipal)
	require.NotNil(t, ps.Prop.CalendarHomeSetHref)
	assert.Equal(t, homeSetPath, *ps.Prop.CalendarHomeSetHref)
}

func TestPropFindHomeSet(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	createCalendar(t, srv)

	res := propFind(t, srv, homeSetPath, "0", propfindHomeSet)
	require.Equal(t, http.StatusMultiStatus, res.Status, "body: %s", res.Body)
	ms := parseMultiStatus(t, res.Body)
	require.Len(t, ms.Responses, 1, "Depth 0 must only return the home set")

	res = propFind(t, srv, homeSetPath, "1", propfindHomeSet)
	require.Equal(t, http.StatusMultiStatus, res.Status, "body: %s", res.Body)
	ms = parseMultiStatus(t, res.Body)
	require.Len(t, ms.Responses, 2, "Depth 1 must return the home set and its calendars")

	assert.ElementsMatch(t, []string{
		path.Clean(homeSetPath),
		path.Clean(calendarPath),
	}, hrefs(ms))
}

func TestPropFindCalendar(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	createCalendar(t, srv)

	res := propFind(t, srv, calendarPath, "0", propfindCalendar)
	require.Equal(t, http.StatusMultiStatus, res.Status, "body: %s", res.Body)

	ms := parseMultiStatus(t, res.Body)
	require.Len(t, ms.Responses, 1)

	ps := ms.Responses[0].requirePropStat(t, http.StatusOK)
	assert.NotNil(t, ps.Prop.ResourceType.Collection)
	assert.NotNil(t, ps.Prop.ResourceType.Calendar)

	components := make([]string, 0, len(ps.Prop.SupportedComponents))
	for _, comp := range ps.Prop.SupportedComponents {
		components = append(components, comp.Name)
	}

	assert.ElementsMatch(t, []string{"VEVENT"}, components)
}

func TestPropFindCalendarObject(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	createCalendar(t, srv)

	put := putCalendarObject(t, srv, objectPath, teapotEvent1)
	require.Equal(t, http.StatusCreated, put.Status, "body: %s", put.Body)

	get := doRequest(t, srv, http.MethodGet, objectPath, "", nil)
	require.Equal(t, http.StatusOK, get.Status)

	res := propFind(t, srv, objectPath, "0", propfindCalendarObject)
	require.Equal(t, http.StatusMultiStatus, res.Status, "body: %s", res.Body)

	ms := parseMultiStatus(t, res.Body)
	require.Len(t, ms.Responses, 1)

	ps := ms.Responses[0].requirePropStat(t, http.StatusOK)
	require.NotNil(t, ps.Prop.GetETag)
	assert.Equal(t, put.Header.Get(headerETag), *ps.Prop.GetETag)
	require.NotNil(t, ps.Prop.GetContentType)
	assert.Equal(t, contentTypeCalendar, *ps.Prop.GetContentType)
	require.NotNil(t, ps.Prop.GetContentLength)
	assert.Equal(t, int64(len(get.Body)), *ps.Prop.GetContentLength)
	require.NotNil(t, ps.Prop.GetLastModified)
	assert.NotEmpty(t, *ps.Prop.GetLastModified)
	require.NotNil(t, ps.Prop.CalendarData)
	assert.Contains(t, *ps.Prop.CalendarData, "SUMMARY:Put Event")

	// displayname is not supported on calendar objects.
	_, ok := ms.Responses[0].propStat(http.StatusNotFound)
	assert.True(t, ok)
}

func TestPropFindCalendarDepth(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	createCalendar(t, srv)

	res := putCalendarObject(t, srv, objectPath, teapotEvent1)
	require.Equal(t, http.StatusCreated, res.Status, "body: %s", res.Body)

	res = propFind(t, srv, calendarPath, "0", propfindCalendar)
	require.Equal(t, http.StatusMultiStatus, res.Status)
	ms := parseMultiStatus(t, res.Body)
	require.Len(t, ms.Responses, 1, "Depth 0 must not return calendar objects")

	res = propFind(t, srv, calendarPath, "1", propfindCalendar)
	require.Equal(t, http.StatusMultiStatus, res.Status)
	ms = parseMultiStatus(t, res.Body)
	require.Len(t, ms.Responses, 2, "Depth 1 must return the calendar and its objects")
	assert.ElementsMatch(t, []string{
		path.Clean(calendarPath),
		path.Clean(objectPath),
	}, hrefs(ms))
}

func TestPropFindUnknownProperty(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)

	res := propFind(t, srv, principalPath, "0", propfindUnknownProperty)
	require.Equal(t, http.StatusMultiStatus, res.Status, "body: %s", res.Body)

	ms := parseMultiStatus(t, res.Body)
	require.Len(t, ms.Responses, 1)

	_, ok := ms.Responses[0].propStat(http.StatusNotFound)
	assert.True(t, ok, "unknown properties must be reported in a 404 propstat")
}

func TestPropFindMalformedXML(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)

	res := propFind(t, srv, principalPath, "0", `<D:propfind xmlns:D="DAV:"><D:prop>`)
	assert.Equal(t, http.StatusBadRequest, res.Status)
}

func seedRFCFixtures(t *testing.T, srv *httptest.Server) {
	t.Helper()

	fixtures := []struct {
		name string
		data string
	}{
		{"event-single.ics", rfc4791EventSingle},
		{"event-recurring.ics", rfc4791EventRecurring},
		{"event-attendees.ics", rfc4791EventAttendees},
		{"todo-due-jan-4.ics", rfc4791TodoDueJan4},
		{"todo-due-jan-6.ics", rfc4791TodoDueJan6},
		{"todo-completed.ics", rfc4791TodoCompleted},
		{"todo-cancelled.ics", rfc4791TodoCancelled},
		{"freebusy.ics", rfc4791Freebusy},
	}

	for _, fixture := range fixtures {
		reqPath := calendarPath + fixture.name

		res := putCalendarObject(t, srv, reqPath, fixture.data)
		require.Equal(t, http.StatusCreated, res.Status, "path: %s, body: %s", reqPath, res.Body)
	}
}

func TestReportCalendarQuery(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want []string
	}{
		{
			name: "uid",
			body: reportCalendarQueryUID,
			want: []string{
				calendarPath + "event-attendees.ics",
			},
		},
		{
			name: "events-only",
			body: reportCalendarQueryEventsOnly,
			want: []string{
				calendarPath + "event-single.ics",
				calendarPath + "event-recurring.ics",
				calendarPath + "event-attendees.ics",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := newTestServer(t)
			createCalendar(t, srv)
			seedRFCFixtures(t, srv)

			res := report(t, srv, calendarPath, tt.body)
			require.Equal(t, http.StatusMultiStatus, res.Status, "body: %s", res.Body)

			ms := parseMultiStatus(t, res.Body)
			assert.ElementsMatch(t, tt.want, hrefs(ms))

			for _, resp := range ms.Responses {
				ps := resp.requirePropStat(t, http.StatusOK)
				assert.NotNil(t, ps.Prop.GetETag)
				assert.NotNil(t, ps.Prop.CalendarData)
			}
		})
	}
}

func TestReportMultiget(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	createCalendar(t, srv)
	seedRFCFixtures(t, srv)

	res := report(t, srv, calendarPath, reportCalendarMultiget)
	require.Equal(t, http.StatusMultiStatus, res.Status, "body: %s", res.Body)

	ms := parseMultiStatus(t, res.Body)
	require.Len(t, ms.Responses, 3)

	byHref := map[string]multiStatusResponse{}
	for _, resp := range ms.Responses {
		byHref[path.Clean(resp.Href)] = resp
	}

	for _, name := range []string{"event-single.ics", "event-attendees.ics"} {
		resp, ok := byHref[calendarPath+name]
		require.True(t, ok, "missing response for %s", name)

		ps := resp.requirePropStat(t, http.StatusOK)
		assert.NotNil(t, ps.Prop.GetETag)
		require.NotNil(t, ps.Prop.CalendarData)
		assert.Contains(t, *ps.Prop.CalendarData, "BEGIN:VCALENDAR")
	}

	missing, ok := byHref[calendarPath+"mtg1.ics"]
	require.True(t, ok, "missing response for non-existent object")
	assert.Equal(t, http.StatusNotFound, missing.Status.Code)
}

func TestReportUnsupportedRoot(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	createCalendar(t, srv)

	res := report(t, srv, calendarPath, reportFreeBusy)
	assert.Equal(t, http.StatusBadRequest, res.Status, "body: %s", res.Body)
}

func TestUnsupportedMethods(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	createCalendar(t, srv)

	tests := []struct {
		name    string
		method  string
		body    string
		headers map[string]string
		want    int
	}{
		{
			name:    "proppatch",
			method:  "PROPPATCH",
			body:    proppatchPropertyUpdate,
			headers: map[string]string{headerContentType: contentTypeXML},
			want:    http.StatusNotImplemented,
		},
		{
			name:    "copy",
			method:  "COPY",
			headers: map[string]string{"Destination": srv.URL + calendarPath + "copy.ics"},
			want:    http.StatusNotImplemented,
		},
		{
			name:    "move",
			method:  "MOVE",
			headers: map[string]string{"Destination": srv.URL + calendarPath + "moved.ics"},
			want:    http.StatusNotImplemented,
		},
		{
			name:   "lock",
			method: "LOCK",
			want:   http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			res := doRequest(t, srv, tt.method, calendarPath, tt.body, tt.headers)
			assert.Equal(t, tt.want, res.Status, "body: %s", res.Body)
		})
	}
}
