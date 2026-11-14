package caldavtest

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAppleOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		method  string
		path    string
		body    string
		depth   string
		wantDAV bool
	}{
		// Ported from:
		// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/options.xml#L31-L47
		{
			name:    "options on calendar home",
			method:  http.MethodOptions,
			path:    calendarHome(userOne),
			wantDAV: true,
		},
		// Ported from:
		// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/options.xml#L48-L64
		// The Apple description says principal, but the script requests calendar home.
		{
			name:    "options on principal",
			method:  http.MethodOptions,
			path:    calendarHome(userOne),
			wantDAV: true,
		},
		// Ported from:
		// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/options.xml#L68-L92
		{
			name:    "propfind on calendar home",
			method:  "PROPFIND",
			path:    calendarHome(userOne),
			body:    loadAppleFixture(t, "Resource/Common/PROPFIND/props.xml", nil),
			depth:   "0",
			wantDAV: false,
		},
		// Ported from:
		// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/options.xml#L93-L117
		// The Apple description says principal, but the script requests calendar home.
		{
			name:    "propfind on principal",
			method:  "PROPFIND",
			path:    calendarHome(userOne),
			body:    loadAppleFixture(t, "Resource/Common/PROPFIND/props.xml", nil),
			depth:   "0",
			wantDAV: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := newTestServer(t)

			res := doRequest(
				t,
				srv,
				tt.method,
				tt.path,
				withContentType(xmlContentType),
				withBody(tt.body),
				withDepth(tt.depth),
			)

			assert.GreaterOrEqual(t, res.Status, 200)
			assert.Less(t, res.Status, 300)
			assert.Equal(t, tt.wantDAV, res.Header.Get("DAV") != "")
		})
	}
}

// Not a direct port: the Apple descriptions mention the principal, but the
// Apple script requests the calendar home. This test checks the principal path.
func TestOptionsAndPropFindOnPrincipal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		method  string
		body    string
		depth   string
		wantDAV bool
	}{
		// OPTIONS on the principal path, as the Apple descriptions state.
		{
			name:    "options",
			method:  http.MethodOptions,
			wantDAV: true,
		},
		// PROPFIND on the principal path, as the Apple descriptions state.
		{
			name:    "propfind",
			method:  "PROPFIND",
			body:    loadAppleFixture(t, "Resource/Common/PROPFIND/props.xml", nil),
			depth:   "0",
			wantDAV: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := newTestServer(t)

			res := doRequest(
				t,
				srv,
				tt.method,
				principal(userOne),
				withContentType(xmlContentType),
				withBody(tt.body),
				withDepth(tt.depth),
			)

			assert.GreaterOrEqual(t, res.Status, 200)
			assert.Less(t, res.Status, 300)
			assert.Equal(t, tt.wantDAV, res.Header.Get("DAV") != "")
		})
	}
}
