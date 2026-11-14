package caldavtest

import (
	"encoding/xml"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func assertCollectionResponse(t *testing.T, res httpResponse, path string, calendar bool) {
	t.Helper()

	require.Equal(t, http.StatusMultiStatus, res.Status, "body: %s", res.Body)

	prop := assertPropstat(t, res.Body, path, http.StatusOK)
	assertResourceType(t, prop, calendar)
}

func assertNotFound(t *testing.T, res httpResponse) {
	t.Helper()

	require.Equal(t, http.StatusNotFound, res.Status, "body: %s", res.Body)
}

func TestApplePropfind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    string
		depth   string
		fixture string
		skip    string
		check   func(t *testing.T, res httpResponse)
	}{
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/propfind.xml#L29-L46
			name:    "home basic properties depth zero",
			path:    calendarHome(userOne),
			depth:   "0",
			fixture: fixturePropfinds1,
			check: func(t *testing.T, res httpResponse) {
				assertCollectionResponse(t, res, calendarHome(userOne), false)
			},
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/propfind.xml#L47-L64
			name:    "home basic properties depth one",
			path:    calendarHome(userOne),
			depth:   "1",
			fixture: fixturePropfinds1,
			check: func(t *testing.T, res httpResponse) {
				assertCollectionResponse(t, res, calendarHome(userOne), false)
				assertCollectionResponse(t, res, calendarPath(userOne), true)
				assertCollectionResponse(t, res, tasksPath(userOne), true)
			},
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/propfind.xml#L65-L86
			name:    "home basic properties depth infinity",
			path:    calendarHome(userOne),
			depth:   "infinity",
			fixture: fixturePropfinds1,
			skip:    skipDepthInfinity,
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/propfind.xml#L87-L108
			name:    "home getcontentlength zero",
			path:    calendarHome(userOne),
			depth:   "0",
			fixture: "Resource/CalDAV/propfinds/7.txt",
			skip:    "Apple compatibility: RFC 4918 14.4 does not require getcontentlength on collections.",
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/propfind.xml#L112-L129
			name:    "calendar basic properties depth zero",
			path:    calendarPath(userOne),
			depth:   "0",
			fixture: fixturePropfinds1,
			check: func(t *testing.T, res httpResponse) {
				assertCollectionResponse(t, res, calendarPath(userOne), true)
			},
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/propfind.xml#L130-L147
			name:    "calendar basic properties depth one",
			path:    calendarPath(userOne),
			depth:   "1",
			fixture: fixturePropfinds1,
			check: func(t *testing.T, res httpResponse) {
				assertCollectionResponse(t, res, calendarPath(userOne), true)
			},
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/propfind.xml#L148-L169
			name:    "calendar basic properties depth infinity",
			path:    calendarPath(userOne),
			depth:   "infinity",
			fixture: fixturePropfinds1,
			skip:    skipDepthInfinity,
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/propfind.xml#L170-L191
			name:    "calendar getcontentlength zero",
			path:    calendarPath(userOne),
			depth:   "0",
			fixture: "Resource/CalDAV/propfinds/7.txt",
			skip:    "Apple compatibility: RFC 4918 14.4 does not require getcontentlength on collections.",
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/propfind.xml#L192-L213
			name:    "missing calendar",
			path:    calendarHome(userOne) + "foobar/",
			depth:   "1",
			fixture: fixturePropfinds1,
			check:   assertNotFound,
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/propfind.xml#L214-L235
			name:    "missing calendar child",
			path:    calendarHome(userOne) + "foobar/foo.ics",
			depth:   "1",
			fixture: fixturePropfinds1,
			check:   assertNotFound,
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/propfind.xml#L236-L257
			name:    "missing calendar child collection",
			path:    calendarHome(userOne) + "foobar/example/",
			depth:   "1",
			fixture: fixturePropfinds1,
			check:   assertNotFound,
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/propfind.xml#L262-L279
			name:    "property names depth zero",
			path:    calendarPath(userOne),
			depth:   "0",
			fixture: fixturePropfinds2,
			check: func(t *testing.T, res httpResponse) {
				require.Equal(t, http.StatusMultiStatus, res.Status, "body: %s", res.Body)
				assertPropNames(t, res.Body, calendarPath(userOne), http.StatusOK,
					xml.Name{Space: davNamespace, Local: "resourcetype"})
			},
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/propfind.xml#L280-L297
			name:    "property names depth one",
			path:    calendarPath(userOne),
			depth:   "1",
			fixture: fixturePropfinds2,
			check: func(t *testing.T, res httpResponse) {
				require.Equal(t, http.StatusMultiStatus, res.Status, "body: %s", res.Body)
				assertPropNames(t, res.Body, calendarPath(userOne), http.StatusOK,
					xml.Name{Space: davNamespace, Local: "resourcetype"})
			},
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/propfind.xml#L298-L319
			name:    "property names depth infinity",
			path:    calendarPath(userOne),
			depth:   "infinity",
			fixture: fixturePropfinds2,
			skip:    skipDepthInfinity,
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/propfind.xml#L323-L340
			name:    "all properties depth zero",
			path:    calendarPath(userOne),
			depth:   "0",
			fixture: fixturePropfinds3,
			check: func(t *testing.T, res httpResponse) {
				assertCollectionResponse(t, res, calendarPath(userOne), true)
			},
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/propfind.xml#L341-L358
			name:    "all properties depth one",
			path:    calendarPath(userOne),
			depth:   "1",
			fixture: fixturePropfinds3,
			check: func(t *testing.T, res httpResponse) {
				assertCollectionResponse(t, res, calendarPath(userOne), true)
			},
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/propfind.xml#L359-L380
			name:    "all properties depth infinity",
			path:    calendarPath(userOne),
			depth:   "infinity",
			fixture: fixturePropfinds3,
			skip:    skipDepthInfinity,
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/propfind.xml#L384-L405
			name:    "invalid elements",
			path:    calendarPath(userOne),
			depth:   "0",
			fixture: "Resource/CalDAV/propfinds/4.txt",
			skip:    "RFC 4918 9.1: PROPFIND with both allprop and propname is invalid and should fail.",
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/propfind.xml#L406-L427
			name:    "unknown element",
			path:    calendarPath(userOne),
			depth:   "0",
			fixture: "Resource/CalDAV/propfinds/5.txt",
			skip:    "RFC 4918 9.1: PROPFIND with an unknown request element should fail.",
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/propfind.xml#L428-L445
			// Expected property set comes from Resource/CalDAV/propfinds/6.txt.
			name:    "one unknown property",
			path:    calendarPath(userOne),
			depth:   "0",
			fixture: "Resource/CalDAV/propfinds/6.txt",
			check: func(t *testing.T, res httpResponse) {
				require.Equal(t, http.StatusMultiStatus, res.Status, "body: %s", res.Body)
				assertPropNames(t, res.Body, calendarPath(userOne), http.StatusOK,
					xml.Name{Space: davNamespace, Local: "resourcetype"})
				assertPropNames(t, res.Body, calendarPath(userOne), http.StatusNotFound,
					xml.Name{Space: davNamespace, Local: "undefined"})
			},
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/propfind.xml#L448-L578
			name:    "depth infinity disabled",
			path:    calendarPath(userOne),
			depth:   "infinity",
			fixture: "Resource/Common/PROPFIND/resourcetype.xml",
			skip:    skipDepthInfinity,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.skip != "" {
				t.Skip(tt.skip)
			}

			srv := newTestServer(t)

			res := propFind(
				t,
				srv,
				tt.path,
				withDepth(tt.depth),
				withBody(loadAppleFixture(t, tt.fixture, nil)),
			)

			if tt.check != nil {
				tt.check(t, res)
				return
			}

			require.Equal(t, http.StatusMultiStatus, res.Status, "body: %s", res.Body)
		})
	}
}
