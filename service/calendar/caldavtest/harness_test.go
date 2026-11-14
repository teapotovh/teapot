package caldavtest

import (
	"context"
	"embed"
	"encoding/xml"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/teapotovh/teapot/lib/httpauth"
	"github.com/teapotovh/teapot/lib/ldap"
	"github.com/teapotovh/teapot/lib/webdav"
	"github.com/teapotovh/teapot/lib/webdav/caldav"
	"github.com/teapotovh/teapot/service/calendar/backend"
	"github.com/teapotovh/teapot/service/calendar/store"

	_ "time/tzdata"
)

const (
	userOne   = "user01"
	userTwo   = "user02"
	userThree = "user03"

	calendarName = "calendar"
	tasksName    = "tasks"

	xmlContentType      = "application/xml; charset=utf-8"
	calendarContentType = "text/calendar; charset=utf-8"

	davNamespace    = "DAV:"
	caldavNamespace = "urn:ietf:params:xml:ns:caldav"
)

var testUsers = map[string]string{
	userOne:   userOne,
	userTwo:   userTwo,
	userThree: userThree,
}

//go:embed testdata/apple
var appleFixtures embed.FS

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

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	logger := slog.New(slog.DiscardHandler)

	st := store.NewMem()
	b := backend.NewBackend(st, logger.With("component", "backend"))

	var handler http.Handler = &caldav.Handler{
		Prefix:  "/",
		Backend: b,
	}

	auth := httpauth.NewBasicAuth(
		&fakeAuthenticator{users: testUsers},
		httpauth.DefaultBasicAuthErrorHandler,
		logger.With("component", "auth"),
	)

	handler = auth.Required(handler)
	handler = auth.Middleware(handler)

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	for user := range testUsers {
		for _, name := range []string{calendarName, tasksName} {
			res := createCalendar(t, srv, user, calendarHome(user)+name+"/")
			require.Equal(t, http.StatusCreated, res.Status, "body: %s", res.Body)
		}
	}

	return srv
}

func principal(user string) string {
	return "/" + user
}

func calendarHome(user string) string {
	return principal(user) + "/calendars/"
}

func calendarPath(user string) string {
	return calendarHome(user) + calendarName + "/"
}

func tasksPath(user string) string {
	return calendarHome(user) + tasksName + "/"
}

type requestConfig struct {
	user        string
	auth        bool
	body        string
	contentType string
	depth       string
	headers     map[string]string
}

type requestOption func(*requestConfig)

func asUser(user string) requestOption {
	return func(config *requestConfig) {
		config.user = user
	}
}

func withoutAuth() requestOption {
	return func(config *requestConfig) {
		config.auth = false
	}
}

func withBody(body string) requestOption {
	return func(config *requestConfig) {
		config.body = body
	}
}

func withContentType(contentType string) requestOption {
	return func(config *requestConfig) {
		config.contentType = contentType
	}
}

func withDepth(depth string) requestOption {
	return func(config *requestConfig) {
		config.depth = depth
	}
}

func withHeader(name string, value string) requestOption {
	return func(config *requestConfig) {
		if config.headers == nil {
			config.headers = map[string]string{}
		}

		config.headers[name] = value
	}
}

func headerTokens(value string) []string {
	tokens := strings.Split(value, ",")
	for i := range tokens {
		tokens[i] = strings.TrimSpace(tokens[i])
	}

	return tokens
}

type httpResponse struct {
	Status int
	Header http.Header
	Body   string
}

func doRequest(
	t *testing.T,
	srv *httptest.Server,
	method string,
	reqPath string,
	options ...requestOption,
) httpResponse {
	t.Helper()

	config := requestConfig{
		user: userOne,
		auth: true,
	}

	for _, option := range options {
		option(&config)
	}

	var body io.Reader
	if config.body != "" {
		body = strings.NewReader(config.body)
	}

	req, err := http.NewRequest(method, srv.URL+reqPath, body)
	require.NoError(t, err)

	if config.auth {
		req.SetBasicAuth(config.user, testUsers[config.user])
	}

	if config.contentType != "" {
		req.Header.Set("Content-Type", config.contentType)
	}

	if config.depth != "" {
		req.Header.Set("Depth", config.depth)
	}

	for name, value := range config.headers {
		req.Header.Set(name, value)
	}

	resp, err := srv.Client().Do(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return httpResponse{
		Status: resp.StatusCode,
		Header: resp.Header,
		Body:   string(respBody),
	}
}

func createCalendar(t *testing.T, srv *httptest.Server, user string, path string) httpResponse {
	t.Helper()

	return doRequest(t, srv, webdav.MethodMkcalendar, path, asUser(user), withContentType(xmlContentType))
}

func putObject(t *testing.T, srv *httptest.Server, user string, path string, data string) httpResponse {
	t.Helper()

	return doRequest(
		t,
		srv,
		http.MethodPut,
		path,
		asUser(user),
		withContentType(calendarContentType),
		withBody(data),
	)
}

func deleteObject(t *testing.T, srv *httptest.Server, user string, path string) httpResponse {
	t.Helper()

	return doRequest(t, srv, http.MethodDelete, path, asUser(user))
}

func propFind(t *testing.T, srv *httptest.Server, path string, options ...requestOption) httpResponse {
	t.Helper()

	options = append([]requestOption{withContentType(xmlContentType)}, options...)

	return doRequest(t, srv, webdav.MethodPropFind, path, options...)
}

func report(
	t *testing.T,
	srv *httptest.Server,
	path string,
	body string,
	options ...requestOption,
) httpResponse {
	t.Helper()

	options = append([]requestOption{withContentType(xmlContentType), withBody(body)}, options...)

	return doRequest(t, srv, webdav.MethodReport, path, options...)
}

type multiStatus struct {
	XMLName   xml.Name              `xml:"DAV: multistatus"`
	Responses []multiStatusResponse `xml:"DAV: response"`
}

type multiStatusResponse struct {
	Hrefs     []string              `xml:"DAV: href"`
	Status    string                `xml:"DAV: status"`
	PropStats []multiStatusPropstat `xml:"DAV: propstat"`
}

type multiStatusPropstat struct {
	Prop   multiStatusProp `xml:"DAV: prop"`
	Status string          `xml:"DAV: status"`
}

type multiStatusProp struct {
	SupportedReportSet  *supportedReportSet    `xml:"DAV: supported-report-set"`
	GetContentLength    *string                `xml:"DAV: getcontentlength"`
	GetContentType      *string                `xml:"DAV: getcontenttype"`
	GetETag             *string                `xml:"DAV: getetag"`
	DisplayName         *string                `xml:"DAV: displayname"`
	CalendarData        *string                `xml:"urn:ietf:params:xml:ns:caldav calendar-data"`
	CalendarDescription *string                `xml:"urn:ietf:params:xml:ns:caldav calendar-description"`
	MaxResourceSize     *string                `xml:"urn:ietf:params:xml:ns:caldav max-resource-size"`
	ResourceType        *davResourceType       `xml:"DAV: resourcetype"`
	SupportedComponents *supportedComponentSet `xml:"urn:ietf:params:xml:ns:caldav supported-calendar-component-set"`
}

type davResourceType struct {
	Collection *struct{} `xml:"DAV: collection"`
	Calendar   *struct{} `xml:"urn:ietf:params:xml:ns:caldav calendar"`
}

type supportedComponentSet struct {
	Comp []struct {
		Name string `xml:"name,attr"`
	} `xml:"urn:ietf:params:xml:ns:caldav comp"`
}

type supportedReportSet struct {
	Reports []supportedReport `xml:"DAV: supported-report"`
}

type supportedReport struct {
	Report reportNames `xml:"DAV: report"`
}

type reportNames struct {
	Names []xml.Name
}

func (r *reportNames) UnmarshalXML(decoder *xml.Decoder, _ xml.StartElement) error {
	for {
		token, err := decoder.Token()
		if err != nil {
			return err
		}

		switch token := token.(type) {
		case xml.StartElement:
			r.Names = append(r.Names, token.Name)

			if err := decoder.Skip(); err != nil {
				return err
			}
		case xml.EndElement:
			return nil
		}
	}
}

func (set *supportedReportSet) names() []xml.Name {
	var names []xml.Name

	for _, report := range set.Reports {
		names = append(names, report.Report.Names...)
	}

	return names
}

type davError struct {
	Names []xml.Name
}

func (e *davError) UnmarshalXML(decoder *xml.Decoder, _ xml.StartElement) error {
	for {
		token, err := decoder.Token()
		if err != nil {
			return err
		}

		switch token := token.(type) {
		case xml.StartElement:
			e.Names = append(e.Names, token.Name)

			if err := decoder.Skip(); err != nil {
				return err
			}
		case xml.EndElement:
			return nil
		}
	}
}

func parseMultiStatus(t *testing.T, body string) multiStatus {
	t.Helper()

	var ms multiStatus
	require.NoError(t, xml.Unmarshal([]byte(body), &ms))

	return ms
}

func (ms multiStatus) findResponse(href string) (multiStatusResponse, bool) {
	for _, response := range ms.Responses {
		for _, candidate := range response.Hrefs {
			if strings.TrimSuffix(candidate, "/") == strings.TrimSuffix(href, "/") {
				return response, true
			}
		}
	}

	return multiStatusResponse{}, false
}

func (response multiStatusResponse) findPropstat(t *testing.T, status int) (multiStatusPropstat, bool) {
	t.Helper()

	for _, propstat := range response.PropStats {
		fields := strings.Fields(propstat.Status)
		require.GreaterOrEqual(t, len(fields), 2)

		code, err := strconv.Atoi(fields[1])
		require.NoError(t, err)

		if code == status {
			return propstat, true
		}
	}

	return multiStatusPropstat{}, false
}

func parseDAVError(t *testing.T, body string) davError {
	t.Helper()

	var davErr davError
	require.NoError(t, xml.Unmarshal([]byte(body), &davErr))

	return davErr
}

func hasName(names []xml.Name, space string, local string) bool {
	for _, name := range names {
		if name.Space == space && name.Local == local {
			return true
		}
	}

	return false
}

var appleFixtureSubstitutions = map[string]string{
	"$uid1:":          "teapot-uid-1",
	"$uid2:":          "teapot-uid-2",
	"$uid3:":          "teapot-uid-3",
	"$uid4:":          "teapot-uid-4",
	"$uid5:":          "teapot-uid-5",
	"$uid6:":          "teapot-uid-6",
	"$uidrandom:":     "teapot-random-uid",
	"$userid1:":       userOne,
	"$userid2:":       userTwo,
	"$pswd1:":         userOne,
	"$pswd2:":         userTwo,
	"$username1:":     userOne,
	"$email1:":        userOne + "@example.com",
	"$principal1:":    principal(userOne),
	"$calendarhome1:": calendarHome(userOne),
	"$calendarpath1:": calendarPath(userOne),
	"$taskspath1:":    tasksPath(userOne),
}

func appleSubstitutionValues(substitutions map[string]string) map[string]string {
	values := make(map[string]string, len(appleFixtureSubstitutions)+len(substitutions))

	maps.Copy(values, appleFixtureSubstitutions)

	maps.Copy(values, substitutions)

	year := time.Now().Year()
	for offset := -3; offset <= 10; offset++ {
		values["$now.year."+strconv.Itoa(offset)+":"] = strconv.Itoa(year + offset)
	}

	return values
}

func loadAppleFixture(t *testing.T, path string, substitutions map[string]string) string {
	t.Helper()

	data, err := appleFixtures.ReadFile("testdata/apple/" + path)
	require.NoError(t, err)

	return expandAppleValue(string(data), substitutions)
}

func expandAppleValue(value string, substitutions map[string]string) string {
	values := appleSubstitutionValues(substitutions)

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	for _, key := range keys {
		value = strings.ReplaceAll(value, key, values[key])
	}

	return value
}
