package internal

import (
	"encoding/xml"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	daverr "github.com/teapotovh/teapot/lib/webdav/error"
)

func TestStatusMarshalText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status Status
		want   string
	}{
		{name: "known code", status: Status{Code: http.StatusNotFound}, want: "HTTP/1.1 404 Not Found"},
		{name: "custom text", status: Status{Code: 299, Text: "Custom"}, want: "HTTP/1.1 299 Custom"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := tt.status.MarshalText()
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(got))
		})
	}
}

func TestStatusUnmarshalText(t *testing.T) {
	t.Parallel()

	t.Run("valid", func(t *testing.T) {
		t.Parallel()

		var status Status

		require.NoError(t, status.UnmarshalText([]byte("HTTP/1.1 404 Not Found")))
		assert.Equal(t, Status{Code: http.StatusNotFound, Text: "Not Found"}, status)
	})

	t.Run("empty", func(t *testing.T) {
		t.Parallel()

		var status Status

		require.NoError(t, status.UnmarshalText(nil))
		assert.Equal(t, Status{}, status)
	})

	tests := []struct {
		name  string
		input string
	}{
		{name: "missing fields", input: "HTTP/1.1"},
		{name: "invalid code", input: "HTTP/1.1 abc Not Found"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var status Status
			assert.Error(t, status.UnmarshalText([]byte(tt.input)))
		})
	}
}

func TestStatusErr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		status   *Status
		wantCode int
	}{
		{name: "nil status", status: nil, wantCode: 0},
		{name: "ok", status: &Status{Code: http.StatusOK}, wantCode: 0},
		{name: "no content", status: &Status{Code: http.StatusNoContent}, wantCode: 0},
		{name: "multi status", status: &Status{Code: http.StatusMultiStatus}, wantCode: 0},
		{name: "not found", status: &Status{Code: http.StatusNotFound}, wantCode: http.StatusNotFound},
		{
			name:     "internal server error",
			status:   &Status{Code: http.StatusInternalServerError},
			wantCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.status.Err()
			if tt.wantCode == 0 {
				assert.NoError(t, err)

				return
			}

			require.Error(t, err)
			assert.Equal(t, tt.wantCode, daverr.HTTPErrorFromError(err).Code)
		})
	}
}

func TestResponseErr(t *testing.T) {
	t.Parallel()

	errElt := &Error{Raw: []RawXMLValue{*NewRawXMLElement(GetETagName, nil, nil)}}

	tests := []struct {
		name     string
		resp     Response
		wantCode int
		wantElt  *Error
	}{
		{name: "no status", resp: Response{}, wantCode: 0},
		{name: "ok", resp: Response{Status: &Status{Code: http.StatusOK}}, wantCode: 0},
		{name: "not found", resp: Response{Status: &Status{Code: http.StatusNotFound}}, wantCode: http.StatusNotFound},
		{
			name:     "not found with description",
			resp:     Response{Status: &Status{Code: http.StatusNotFound}, ResponseDescription: "no such resource"},
			wantCode: http.StatusNotFound,
		},
		{
			name:     "not found with error element",
			resp:     Response{Status: &Status{Code: http.StatusNotFound}, Error: errElt},
			wantCode: http.StatusNotFound,
			wantElt:  errElt,
		},
		{
			name: "not found with description and error element",
			resp: Response{
				Status:              &Status{Code: http.StatusNotFound},
				ResponseDescription: "no such resource",
				Error:               errElt,
			},
			wantCode: http.StatusNotFound,
			wantElt:  errElt,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.resp.Err()
			if tt.wantCode == 0 {
				assert.NoError(t, err)

				return
			}

			require.Error(t, err)

			httpErr := daverr.HTTPErrorFromError(err)
			assert.Equal(t, tt.wantCode, httpErr.Code)

			var propErr *Error
			if tt.wantElt != nil {
				require.ErrorAs(t, err, &propErr)
				assert.Equal(t, tt.wantElt, propErr)
			}

			if tt.resp.ResponseDescription != "" {
				assert.Contains(t, err.Error(), tt.resp.ResponseDescription)
			}
		})
	}
}

func TestResponseErrUnknownDescription(t *testing.T) {
	t.Parallel()

	resp := Response{
		Status:              &Status{Code: http.StatusNotFound},
		ResponseDescription: "no such resource",
	}

	err := resp.Err()
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnknown)
}

func TestPropStatErr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		propStat PropStat
		wantCode int
	}{
		{name: "ok", propStat: PropStat{Status: Status{Code: http.StatusOK}}, wantCode: 0},
		{
			name:     "forbidden",
			propStat: PropStat{Status: Status{Code: http.StatusForbidden}},
			wantCode: http.StatusForbidden,
		},
		{
			name:     "not found",
			propStat: PropStat{Status: Status{Code: http.StatusNotFound}},
			wantCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.propStat.Status.Err()
			if tt.wantCode == 0 {
				assert.NoError(t, err)

				return
			}

			require.Error(t, err)
			assert.Equal(t, tt.wantCode, daverr.HTTPErrorFromError(err).Code)
		})
	}
}

func TestNewMultiStatus(t *testing.T) {
	t.Parallel()

	ms := NewMultiStatus(*NewOKResponse("/a"), *NewOKResponse("/b"))
	assert.Len(t, ms.Responses, 2)
}

func TestNewOKResponse(t *testing.T) {
	t.Parallel()

	resp := NewOKResponse("/calendars/user")

	require.Len(t, resp.Hrefs, 1)
	assert.Equal(t, "/calendars/user", resp.Hrefs[0].Path)
	require.NotNil(t, resp.Status)
	assert.Equal(t, http.StatusOK, resp.Status.Code)
	assert.NoError(t, resp.Err())

	path, err := resp.Path()
	require.NoError(t, err)
	assert.Equal(t, "/calendars/user", path)
}

func TestNewErrorResponse(t *testing.T) {
	t.Parallel()

	errElt := &Error{Raw: []RawXMLValue{*NewRawXMLElement(GetETagName, nil, nil)}}

	tests := []struct {
		name     string
		err      error
		wantCode int
		wantElt  *Error
	}{
		{
			name:     "plain error",
			err:      errTestFailure,
			wantCode: http.StatusInternalServerError,
		},
		{
			name:     "http error",
			err:      daverr.HTTPErrorf(http.StatusForbidden, "denied"),
			wantCode: http.StatusForbidden,
		},
		{
			name:     "error element",
			err:      errElt,
			wantCode: http.StatusInternalServerError,
			wantElt:  errElt,
		},
		{
			name:     "http error wrapping error element",
			err:      &daverr.HTTPError{Code: http.StatusConflict, Err: errElt},
			wantCode: http.StatusConflict,
			wantElt:  errElt,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resp := NewErrorResponse("/calendars/user", tt.err)

			require.Len(t, resp.Hrefs, 1)
			assert.Equal(t, "/calendars/user", resp.Hrefs[0].Path)
			require.NotNil(t, resp.Status)
			assert.Equal(t, tt.wantCode, resp.Status.Code)
			assert.Equal(t, tt.err.Error(), resp.ResponseDescription)
			assert.Equal(t, tt.wantElt, resp.Error)

			err := resp.Err()
			require.Error(t, err)
			assert.Equal(t, tt.wantCode, daverr.HTTPErrorFromError(err).Code)
		})
	}
}

func TestETag(t *testing.T) {
	t.Parallel()

	etag := ETag("abc123")
	assert.Equal(t, `"abc123"`, etag.String())

	text, err := etag.MarshalText()
	require.NoError(t, err)
	assert.Equal(t, `"abc123"`, string(text))

	var parsed ETag

	require.NoError(t, parsed.UnmarshalText([]byte(`"abc123"`)))
	assert.Equal(t, etag, parsed)

	assert.Error(t, parsed.UnmarshalText([]byte("abc123")))
}

func TestETagXMLRoundTrip(t *testing.T) {
	t.Parallel()

	input := `<getetag xmlns="DAV:">"abc123"</getetag>`

	var etag GetETag

	require.NoError(t, xml.Unmarshal([]byte(input), &etag))
	assert.Equal(t, ETag("abc123"), etag.ETag)

	got, err := xml.Marshal(&etag)
	require.NoError(t, err)
	assert.Equal(t, `<getetag xmlns="DAV:">&#34;abc123&#34;</getetag>`, string(got))

	var unquoted GetETag
	assert.Error(t, xml.Unmarshal([]byte(`<getetag xmlns="DAV:">abc123</getetag>`), &unquoted))
}

func TestResourceType(t *testing.T) {
	t.Parallel()

	rt := NewResourceType(CollectionName, PrincipalName)

	assert.True(t, rt.Is(CollectionName))
	assert.True(t, rt.Is(PrincipalName))
	assert.False(t, rt.Is(DisplayNameName))
	assert.False(t, NewResourceType().Is(CollectionName))

	got, err := xml.Marshal(rt)
	require.NoError(t, err)
	assert.Equal(
		t,
		`<resourcetype xmlns="DAV:"><collection xmlns="DAV:"></collection>`+
			`<principal xmlns="DAV:"></principal></resourcetype>`,
		string(got),
	)

	var decoded ResourceType

	require.NoError(t, xml.Unmarshal(got, &decoded))
	assert.True(t, decoded.Is(CollectionName))
	assert.True(t, decoded.Is(PrincipalName))
}

func TestEncodeAndDecodeProp(t *testing.T) {
	t.Parallel()

	prop, err := EncodeProp(&DisplayName{Name: testDisplayName}, NewResourceType(CollectionName))
	require.NoError(t, err)
	require.Len(t, prop.Raw, 2)

	encoded, err := xml.Marshal(prop)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `<displayname xmlns="DAV:">`+testDisplayName+`</displayname>`)
	assert.Contains(t, string(encoded), `<collection xmlns="DAV:"></collection>`)

	input := `<response xmlns="DAV:">` +
		`<href>/calendars/user</href>` +
		`<propstat><prop><displayname>` + testDisplayName + `</displayname>` +
		`<resourcetype><collection/></resourcetype></prop>` +
		`<status>HTTP/1.1 200 OK</status></propstat>` +
		`</response>`

	var resp Response

	require.NoError(t, xml.Unmarshal([]byte(input), &resp))

	var name DisplayName

	require.NoError(t, resp.DecodeProp(&name))
	assert.Equal(t, testDisplayName, name.Name)

	var rt ResourceType

	require.NoError(t, resp.DecodeProp(&rt))
	assert.True(t, rt.Is(CollectionName))

	var etag GetETag

	err = resp.DecodeProp(&etag)
	require.Error(t, err)
	assert.True(t, daverr.IsNotFound(err))
}

func TestResponseDecodePropErrorStatus(t *testing.T) {
	t.Parallel()

	input := `<response xmlns="DAV:">` +
		`<href>/calendars/user</href>` +
		`<propstat><prop><displayname>` + testDisplayName + `</displayname></prop>` +
		`<status>HTTP/1.1 403 Forbidden</status></propstat>` +
		`</response>`

	var resp Response

	require.NoError(t, xml.Unmarshal([]byte(input), &resp))

	var name DisplayName

	err := resp.DecodeProp(&name)
	require.Error(t, err)
	assert.Equal(t, http.StatusForbidden, daverr.HTTPErrorFromError(err).Code)
}

func TestPropGet(t *testing.T) {
	t.Parallel()

	prop := &Prop{Raw: []RawXMLValue{*NewRawXMLElement(DisplayNameName, nil, nil)}}

	raw := prop.Get(DisplayNameName)
	require.NotNil(t, raw)

	name, ok := raw.XMLName()
	require.True(t, ok)
	assert.Equal(t, DisplayNameName, name)

	assert.Nil(t, prop.Get(ResourceTypeName))
}

func TestNewPropNamePropFind(t *testing.T) {
	t.Parallel()

	propfind := NewPropNamePropFind(DisplayNameName, ResourceTypeName)

	require.NotNil(t, propfind.Prop)
	require.Len(t, propfind.Prop.Raw, 2)

	name, ok := propfind.Prop.Raw[0].XMLName()
	require.True(t, ok)
	assert.Equal(t, DisplayNameName, name)
}

func TestPropFindDecode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		input        string
		wantPropName bool
		wantAllProp  bool
		wantNames    []xml.Name
	}{
		{
			name:         "propname",
			input:        `<propfind xmlns="DAV:"><propname/></propfind>`,
			wantPropName: true,
		},
		{
			name:        "allprop",
			input:       `<propfind xmlns="DAV:"><allprop/></propfind>`,
			wantAllProp: true,
		},
		{
			name:      "empty prop",
			input:     `<propfind xmlns="DAV:"><prop></prop></propfind>`,
			wantNames: []xml.Name{},
		},
		{
			name:      "prop with names",
			input:     `<propfind xmlns="DAV:"><prop><displayname/><resourcetype/></prop></propfind>`,
			wantNames: []xml.Name{DisplayNameName, ResourceTypeName},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var propfind PropFind

			require.NoError(t, xml.Unmarshal([]byte(tt.input), &propfind))
			assert.Equal(t, xml.Name{Space: Namespace, Local: "propfind"}, propfind.XMLName)
			assert.Equal(t, tt.wantPropName, propfind.PropName != nil)
			assert.Equal(t, tt.wantAllProp, propfind.AllProp != nil)

			if tt.wantNames == nil {
				assert.Nil(t, propfind.Prop)

				return
			}

			require.NotNil(t, propfind.Prop)
			require.Len(t, propfind.Prop.Raw, len(tt.wantNames))

			for i, name := range tt.wantNames {
				got, ok := propfind.Prop.Raw[i].XMLName()
				require.True(t, ok)
				assert.Equal(t, name, got)
			}
		})
	}
}

func TestPropFindDecodeInclude(t *testing.T) {
	t.Parallel()

	input := `<propfind xmlns="DAV:"><allprop/><include><displayname/></include></propfind>`

	var propfind PropFind

	require.NoError(t, xml.Unmarshal([]byte(input), &propfind))
	require.NotNil(t, propfind.AllProp)
	require.NotNil(t, propfind.Include)
	require.Len(t, propfind.Include.Raw, 1)

	name, ok := propfind.Include.Raw[0].XMLName()
	require.True(t, ok)
	assert.Equal(t, DisplayNameName, name)
}

func TestPropertyUpdateDecode(t *testing.T) {
	t.Parallel()

	input := `<propertyupdate xmlns="DAV:">` +
		`<set><prop><displayname>home</displayname></prop></set>` +
		`<set><prop><resourcetype><collection/></resourcetype></prop></set>` +
		`<remove><prop><getetag/></prop></remove>` +
		`</propertyupdate>`

	var update PropertyUpdate

	require.NoError(t, xml.Unmarshal([]byte(input), &update))
	assert.Equal(t, xml.Name{Space: Namespace, Local: "propertyupdate"}, update.XMLName)
	require.Len(t, update.Set, 2)
	require.Len(t, update.Remove, 1)

	var name DisplayName

	require.NoError(t, update.Set[0].Prop.Decode(&name))
	assert.Equal(t, testDisplayName, name.Name)

	var rt ResourceType

	require.NoError(t, update.Set[1].Prop.Decode(&rt))
	assert.True(t, rt.Is(CollectionName))

	raw := update.Remove[0].Prop.Get(GetETagName)
	require.NotNil(t, raw)

	rawName, ok := raw.XMLName()
	require.True(t, ok)
	assert.Equal(t, GetETagName, rawName)
}
