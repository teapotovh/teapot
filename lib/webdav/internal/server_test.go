package internal

import (
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	daverr "github.com/teapotovh/teapot/lib/webdav/error"
)

func marshalProp(t *testing.T, raw *RawXMLValue) string {
	t.Helper()

	b, err := xml.Marshal(raw)
	require.NoError(t, err)

	return string(b)
}

func propName(t *testing.T, raw *RawXMLValue) xml.Name {
	t.Helper()

	var probe struct {
		XMLName xml.Name
	}

	require.NoError(t, xml.Unmarshal([]byte(marshalProp(t, raw)), &probe))

	return probe.XMLName
}

func findProp(t *testing.T, resp *Response, name xml.Name) (string, int) {
	t.Helper()

	for i := range resp.PropStats {
		propStat := &resp.PropStats[i]

		for j := range propStat.Prop.Raw {
			if propName(t, &propStat.Prop.Raw[j]) == name {
				return marshalProp(t, &propStat.Prop.Raw[j]), propStat.Status.Code
			}
		}
	}

	t.Fatalf("property %v not found in response", name)

	return "", 0
}

func TestNewPropFindResponsePropName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		props     map[xml.Name]PropFindFunc
		wantNames []xml.Name
	}{
		{
			name: "explicit resource type",
			props: map[xml.Name]PropFindFunc{
				DisplayNameName:  PropFindValue(&DisplayName{Name: testDisplayName}),
				ResourceTypeName: PropFindValue(NewResourceType(CollectionName)),
			},
			wantNames: []xml.Name{DisplayNameName, ResourceTypeName},
		},
		{
			name: "implicit resource type",
			props: map[xml.Name]PropFindFunc{
				DisplayNameName: PropFindValue(&DisplayName{Name: testDisplayName}),
			},
			wantNames: []xml.Name{DisplayNameName, ResourceTypeName},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resp, err := NewPropFindResponse("/calendars/user", &PropFind{PropName: &struct{}{}}, tt.props)
			require.NoError(t, err)

			require.Len(t, resp.PropStats, 1)
			assert.Equal(t, http.StatusOK, resp.PropStats[0].Status.Code)
			require.Len(t, resp.PropStats[0].Prop.Raw, len(tt.wantNames))

			got := make(map[xml.Name]string, len(tt.wantNames))

			for i := range resp.PropStats[0].Prop.Raw {
				raw := &resp.PropStats[0].Prop.Raw[i]
				got[propName(t, raw)] = marshalProp(t, raw)
			}

			assert.Len(t, got, len(tt.wantNames))

			for _, name := range tt.wantNames {
				body, ok := got[name]
				require.True(t, ok, "missing property %v", name)
				assert.Contains(t, body, "></")
			}
		})
	}
}

func TestNewPropFindResponseAllProp(t *testing.T) {
	t.Parallel()

	props := map[xml.Name]PropFindFunc{
		ResourceTypeName: PropFindValue(NewResourceType(CollectionName)),
		DisplayNameName:  PropFindValue(&DisplayName{Name: testDisplayName}),
	}

	resp, err := NewPropFindResponse("/calendars/user", &PropFind{AllProp: &struct{}{}}, props)
	require.NoError(t, err)

	b, code := findProp(t, resp, DisplayNameName)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, `<displayname xmlns="DAV:">`+testDisplayName+`</displayname>`, b)

	resourceType, code := findProp(t, resp, ResourceTypeName)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(
		t,
		`<resourcetype xmlns="DAV:"><collection xmlns="DAV:"></collection></resourcetype>`,
		resourceType,
	)

	var rt ResourceType

	require.NoError(t, xml.Unmarshal([]byte(resourceType), &rt))
	assert.True(t, rt.Is(CollectionName))
}

func TestNewPropFindResponseProp(t *testing.T) {
	t.Parallel()

	unknownName := xml.Name{Space: "http://example.com/ns", Local: "unknown"}

	propfind := &PropFind{Prop: &Prop{Raw: xmlNamesToRaw([]xml.Name{
		DisplayNameName,
		GetETagName,
		unknownName,
	})}}
	props := map[xml.Name]PropFindFunc{
		DisplayNameName: PropFindValue(&DisplayName{Name: testDisplayName}),
	}

	resp, err := NewPropFindResponse("/calendars/user", propfind, props)
	require.NoError(t, err)
	assert.NoError(t, resp.Err())

	b, code := findProp(t, resp, DisplayNameName)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, `<displayname xmlns="DAV:">`+testDisplayName+`</displayname>`, b)

	b, code = findProp(t, resp, GetETagName)
	assert.Equal(t, http.StatusNotFound, code)
	assert.Equal(t, `<getetag xmlns="DAV:"></getetag>`, b)

	b, code = findProp(t, resp, unknownName)
	assert.Equal(t, http.StatusNotFound, code)
	assert.Equal(t, `<unknown xmlns="http://example.com/ns"></unknown>`, b)
}

func TestNewPropFindResponseMissingQuery(t *testing.T) {
	t.Parallel()

	_, err := NewPropFindResponse("/calendars/user", &PropFind{}, map[xml.Name]PropFindFunc{})
	require.Error(t, err)
	assert.Equal(t, http.StatusBadRequest, daverr.HTTPErrorFromError(err).Code)
}

func TestDecodeXMLRequest(t *testing.T) {
	t.Parallel()

	const body = `<propfind xmlns="DAV:"><allprop/></propfind>`

	tests := []struct {
		name        string
		contentType string
		body        string
		wantErr     bool
		wantAllProp bool
	}{
		{name: "missing content type", contentType: "", body: body, wantErr: true},
		{name: "wrong content type", contentType: "application/json", body: body, wantErr: true},
		{name: "application xml", contentType: "application/xml", body: body, wantAllProp: true},
		{name: "text xml", contentType: "text/xml", body: body, wantAllProp: true},
		{
			name:        "application xml with charset",
			contentType: "application/xml; charset=utf-8",
			body:        body,
			wantAllProp: true,
		},
		{name: "invalid xml", contentType: "application/xml", body: `<propfind`, wantErr: true},
		{name: "empty body", contentType: "application/xml", body: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodPost, "/calendars/user", strings.NewReader(tt.body))
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}

			var propfind PropFind

			err := DecodeXMLRequest(req, &propfind)
			if tt.wantErr {
				require.Error(t, err)
				assert.Equal(t, http.StatusBadRequest, daverr.HTTPErrorFromError(err).Code)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantAllProp, propfind.AllProp != nil)
		})
	}
}

func TestIsRequestBodyEmpty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body io.Reader
		want bool
	}{
		{name: "no body", body: http.NoBody, want: true},
		{name: "empty reader", body: strings.NewReader(""), want: true},
		{name: "whitespace only", body: strings.NewReader("  \n\t"), want: false},
		{name: "non empty", body: strings.NewReader("body"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodPost, "/", tt.body)
			assert.Equal(t, tt.want, IsRequestBodyEmpty(req))
		})
	}
}

func TestServeErrorPlainError(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()

	ServeError(w, errTestFailure)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "boom\n", w.Body.String())
	assert.Contains(t, w.Header().Get("Content-Type"), "text/plain")
}

func TestServeErrorHTTPError(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()

	ServeError(w, daverr.HTTPErrorf(http.StatusNotFound, "no such resource"))

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "no such resource")
}

func TestServeErrorXMLElement(t *testing.T) {
	t.Parallel()

	errElt := &Error{Raw: []RawXMLValue{
		*NewRawXMLElement(xml.Name{Space: "http://example.com/ns", Local: "reason"}, nil, nil),
	}}

	tests := []struct {
		name     string
		err      error
		wantCode int
	}{
		{name: "plain error element", err: errElt, wantCode: http.StatusInternalServerError},
		{
			name:     "http error wrapping element",
			err:      &daverr.HTTPError{Code: http.StatusConflict, Err: errElt},
			wantCode: http.StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			w := httptest.NewRecorder()

			ServeError(w, tt.err)

			assert.Equal(t, tt.wantCode, w.Code)
			assert.Equal(t, `application/xml; charset="utf-8"`, w.Header().Get("Content-Type"))

			body := w.Body.String()
			assert.Contains(t, body, xml.Header)
			assert.Contains(t, body, `<error xmlns="DAV:">`)
			assert.Contains(t, body, `<reason xmlns="http://example.com/ns">`)
		})
	}
}

func TestServeMultiStatus(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()

	ms := NewMultiStatus(
		*NewOKResponse("/calendars/user"),
		*NewErrorResponse("/calendars/user/missing", daverr.HTTPErrorf(http.StatusNotFound, "no such resource")),
	)

	require.NoError(t, ServeMultiStatus(w, ms))

	assert.Equal(t, http.StatusMultiStatus, w.Code)
	assert.Equal(t, `application/xml; charset="utf-8"`, w.Header().Get("Content-Type"))

	body := w.Body.String()
	assert.Contains(t, body, xml.Header)
	assert.Contains(t, body, `<multistatus xmlns="DAV:">`)
	assert.Contains(t, body, `<href>/calendars/user</href>`)
	assert.Contains(t, body, `<status>HTTP/1.1 404 Not Found</status>`)
	assert.Equal(t, 2, strings.Count(body, "<response xmlns="))

	var decoded MultiStatus

	require.NoError(t, xml.Unmarshal(w.Body.Bytes(), &decoded))
	require.Len(t, decoded.Responses, 2)
	assert.NoError(t, decoded.Responses[0].Err())
	assert.Equal(t, http.StatusNotFound, daverr.HTTPErrorFromError(decoded.Responses[1].Err()).Code)
}

type stubBackend struct{}

func (stubBackend) Options(*http.Request) ([]string, []string, error) {
	return nil, []string{http.MethodOptions, MethodPropFind}, nil
}

func (stubBackend) HeadGet(http.ResponseWriter, *http.Request) error { return nil }

func (stubBackend) PropFind(*http.Request, *PropFind, Depth) (*MultiStatus, error) {
	return NewMultiStatus(), nil
}

func (stubBackend) PropPatch(*http.Request, *PropertyUpdate) (*Response, error) {
	return &Response{}, nil
}

func (stubBackend) Put(http.ResponseWriter, *http.Request) error { return nil }

func (stubBackend) Delete(*http.Request) error { return nil }

func (stubBackend) Mkcol(*http.Request) error { return nil }

func (stubBackend) Copy(*http.Request, *Href, bool, bool) (bool, error) { return false, nil }

func (stubBackend) Move(*http.Request, *Href, bool) (bool, error) { return false, nil }

type recordingPropFindBackend struct {
	stubBackend

	called bool
}

func (b *recordingPropFindBackend) PropFind(*http.Request, *PropFind, Depth) (*MultiStatus, error) {
	b.called = true

	return NewMultiStatus(), nil
}

func TestHandlePropfindValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		body      string
		wantCode  int
		wantCalls bool
	}{
		{
			name:     "allprop and propname",
			body:     `<propfind xmlns="DAV:"><allprop/><propname/></propfind>`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "allprop and prop",
			body:     `<propfind xmlns="DAV:"><allprop/><prop><displayname/></prop></propfind>`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "prop and propname",
			body:     `<propfind xmlns="DAV:"><prop><displayname/></prop><propname/></propfind>`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "unknown element",
			body:     `<propfind xmlns="DAV:"><undefined/></propfind>`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:      "allprop with include",
			body:      `<propfind xmlns="DAV:"><allprop/><include><displayname/></include></propfind>`,
			wantCode:  http.StatusMultiStatus,
			wantCalls: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := &recordingPropFindBackend{}
			h := &Handler{Backend: backend}

			req := httptest.NewRequest(MethodPropFind, "/calendars/user", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/xml")

			w := httptest.NewRecorder()

			h.ServeHTTP(w, req)

			assert.Equal(t, tt.wantCode, w.Code)
			assert.Equal(t, tt.wantCalls, backend.called)
		})
	}
}

func TestMethodNotAllowedSetsAllow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		method string
	}{
		{name: "unsupported method", method: MethodLock},
		{name: "unsupported mkcalendar", method: MethodMkcalendar},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(tt.method, "/calendars/user/", nil)
			w := httptest.NewRecorder()

			h := &Handler{Backend: stubBackend{}}
			h.ServeHTTP(w, req)

			assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
			assert.Equal(t, "OPTIONS, PROPFIND", w.Header().Get("Allow"))
		})
	}
}
