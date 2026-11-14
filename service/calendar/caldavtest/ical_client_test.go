package caldavtest

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAppleICalClient(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		body string
		opts []requestOption
		want int
	}{
		// Ported from:
		// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/ical-client.xml#L31-L52
		{
			name: "client 1 principal without auth",
			path: principal(userOne),
			body: loadAppleFixture(t, "Resource/CalDAV/ical-client/client1/1.xml", nil),
			opts: []requestOption{withoutAuth(), withHeader("Depth", "0")},
			want: http.StatusUnauthorized,
		},
		// Ported from:
		// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/ical-client.xml#L53-L74
		{
			name: "client 1 principal with auth",
			path: principal(userOne),
			body: loadAppleFixture(t, "Resource/CalDAV/ical-client/client1/1.xml", nil),
			opts: []requestOption{withHeader("Depth", "0")},
			want: http.StatusMultiStatus,
		},
		// Ported from:
		// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/ical-client.xml#L78-L99
		{
			name: "client 1 calendar home",
			path: calendarHome(userOne),
			body: loadAppleFixture(t, "Resource/CalDAV/ical-client/client1/2.xml", nil),
			opts: []requestOption{withDepth("1")},
			want: http.StatusMultiStatus,
		},
		// Ported from:
		// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/ical-client.xml#L103-L124
		{
			name: "client 2 principal without auth",
			path: principal(userOne),
			body: loadAppleFixture(t, "Resource/CalDAV/ical-client/client2/1.xml", nil),
			opts: []requestOption{withoutAuth(), withHeader("Depth", "0")},
			want: http.StatusUnauthorized,
		},
		// Ported from:
		// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/ical-client.xml#L125-L146
		{
			name: "client 2 principal with auth",
			path: principal(userOne),
			body: loadAppleFixture(t, "Resource/CalDAV/ical-client/client2/1.xml", nil),
			opts: []requestOption{withHeader("Depth", "0")},
			want: http.StatusMultiStatus,
		},
		// Ported from:
		// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/ical-client.xml#L150-L171
		{
			name: "client 2 calendar home",
			path: calendarHome(userOne),
			body: loadAppleFixture(t, "Resource/CalDAV/ical-client/client2/2.xml", nil),
			opts: []requestOption{withDepth("1")},
			want: http.StatusMultiStatus,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := newTestServer(t)

			res := propFind(t, srv, tt.path, append(tt.opts, withBody(tt.body))...)
			assert.Equal(t, tt.want, res.Status, "body: %s", res.Body)

			if tt.want == http.StatusUnauthorized {
				assert.NotEmpty(t, res.Header.Get("WWW-Authenticate"))
			}
		})
	}
}
