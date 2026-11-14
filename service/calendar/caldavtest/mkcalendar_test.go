package caldavtest

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/teapotovh/teapot/lib/webdav"
)

const calendarPropsQuery = `<D:propfind xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav">` +
	`<D:prop>` +
	`<D:resourcetype/>` +
	`<D:displayname/>` +
	`<C:calendar-description/>` +
	`<C:supported-calendar-component-set/>` +
	`<C:max-resource-size/>` +
	`</D:prop></D:propfind>`

func calendarProps(t *testing.T, srv *httptest.Server, path string) multiStatusProp {
	t.Helper()

	res := propFind(t, srv, path, withDepth("0"), withBody(calendarPropsQuery))
	require.Equal(t, http.StatusMultiStatus, res.Status, "body: %s", res.Body)

	prop := assertPropstat(t, res.Body, path, http.StatusOK)
	assert.Nil(t, prop.MaxResourceSize)

	return prop
}

func TestAppleMkcalendar(t *testing.T) {
	t.Parallel()

	t.Run("simple", func(t *testing.T) {
		t.Parallel()

		// Ported from:
		// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/mkcalendar.xml#L29-L42
		srv := newTestServer(t)

		res := createCalendar(t, srv, userOne, calendarHome(userOne)+"caltest1/")
		require.Equal(t, http.StatusCreated, res.Status, "body: %s", res.Body)

		prop := calendarProps(t, srv, calendarHome(userOne)+"caltest1/")
		assertResourceType(t, prop, true)
		assertSupportedComponents(
			t,
			prop,
			[]string{componentVEvent, componentVTodo, componentVJournal, componentVFreeBusy},
		)
		assert.Nil(t, prop.DisplayName)
		assert.Nil(t, prop.CalendarDescription)
	})

	t.Run("existing resource", func(t *testing.T) {
		t.Parallel()

		// Ported from:
		// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/mkcalendar.xml#L43-L56
		t.Skip(
			"RFC 4791 5.3.1: MKCALENDAR on an existing resource must fail with the resource-must-be-null precondition.",
		)

		srv := newTestServer(t)

		res := createCalendar(t, srv, userOne, calendarHome(userOne)+"caltest1/")
		require.Equal(t, http.StatusCreated, res.Status, "body: %s", res.Body)

		res = createCalendar(t, srv, userOne, calendarHome(userOne)+"caltest1/")
		require.Equal(t, http.StatusPreconditionFailed, res.Status, "body: %s", res.Body)

		davErr := parseDAVError(t, res.Body)
		assert.True(t, hasName(davErr.Names, davNamespace, "resource-must-be-null"))
	})

	t.Run("inside another calendar", func(t *testing.T) {
		t.Parallel()

		// Ported from:
		// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/mkcalendar.xml#L57-L70
		t.Skip("RFC 4791 5.3.1: MKCALENDAR nested inside a calendar must fail with calendar-collection-location-ok.")

		srv := newTestServer(t)

		res := createCalendar(t, srv, userOne, calendarHome(userOne)+"caltest1/")
		require.Equal(t, http.StatusCreated, res.Status, "body: %s", res.Body)

		res = createCalendar(t, srv, userOne, calendarHome(userOne)+"caltest1/caltest2/")
		require.Equal(t, http.StatusForbidden, res.Status, "body: %s", res.Body)

		davErr := parseDAVError(t, res.Body)
		assert.True(t, hasName(davErr.Names, caldavNamespace, "calendar-collection-location-ok"))
	})

	t.Run("mkcol inside calendar", func(t *testing.T) {
		t.Parallel()

		// Ported from:
		// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/mkcalendar.xml#L71-L84
		srv := newTestServer(t)

		res := createCalendar(t, srv, userOne, calendarHome(userOne)+"caltest1/")
		require.Equal(t, http.StatusCreated, res.Status, "body: %s", res.Body)

		res = doRequest(t, srv, webdav.MethodMkcol, calendarHome(userOne)+"caltest1/collection/")
		require.Equal(t, http.StatusForbidden, res.Status, "body: %s", res.Body)
	})

	t.Run("with body", func(t *testing.T) {
		t.Parallel()

		// Ported from:
		// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/mkcalendar.xml#L88-L105
		// Expected displayname and description come from Resource/CalDAV/mkcalendar/1.txt.
		srv := newTestServer(t)

		res := doRequest(
			t,
			srv,
			webdav.MethodMkcalendar,
			calendarHome(userOne)+"caltest2/",
			withContentType(xmlContentType),
			withBody(loadAppleFixture(t, fixtureMkcalendar1, nil)),
		)
		require.Equal(t, http.StatusCreated, res.Status, "body: %s", res.Body)

		prop := calendarProps(t, srv, calendarHome(userOne)+"caltest2/")
		assertResourceType(t, prop, true)
		require.NotNil(t, prop.DisplayName)
		assert.Equal(t, "A Calendar", *prop.DisplayName)
		require.NotNil(t, prop.CalendarDescription)
		assert.Equal(t, "First CalDAV calendar.", *prop.CalendarDescription)
		assertSupportedComponents(
			t,
			prop,
			[]string{componentVEvent, componentVTodo, componentVJournal, componentVFreeBusy},
		)
	})

	t.Run("with body on existing resource", func(t *testing.T) {
		t.Parallel()

		// Ported from:
		// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/mkcalendar.xml#L106-L123
		t.Skip(
			"RFC 4791 5.3.1: MKCALENDAR on an existing resource must fail with the resource-must-be-null precondition.",
		)

		srv := newTestServer(t)

		res := doRequest(
			t,
			srv,
			webdav.MethodMkcalendar,
			calendarHome(userOne)+"caltest2/",
			withContentType(xmlContentType),
			withBody(loadAppleFixture(t, fixtureMkcalendar1, nil)),
		)
		require.Equal(t, http.StatusCreated, res.Status, "body: %s", res.Body)

		res = doRequest(
			t,
			srv,
			webdav.MethodMkcalendar,
			calendarHome(userOne)+"caltest2/",
			withContentType(xmlContentType),
			withBody(loadAppleFixture(t, fixtureMkcalendar1, nil)),
		)
		require.Equal(t, http.StatusPreconditionFailed, res.Status, "body: %s", res.Body)

		davErr := parseDAVError(t, res.Body)
		assert.True(t, hasName(davErr.Names, davNamespace, "resource-must-be-null"))
	})

	t.Run("with incorrect body", func(t *testing.T) {
		t.Parallel()

		// Ported from:
		// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/mkcalendar.xml#L124-L154
		t.Skip("RFC 4918 9.3.1: MKCALENDAR must reject properties it cannot set.")

		srv := newTestServer(t)

		res := doRequest(
			t,
			srv,
			webdav.MethodMkcalendar,
			calendarHome(userOne)+"caltest3/",
			withContentType(xmlContentType),
			withBody(loadAppleFixture(t, "Resource/CalDAV/mkcalendar/2.txt", nil)),
		)
		require.Equal(t, http.StatusForbidden, res.Status, "body: %s", res.Body)

		res = doRequest(t, srv, http.MethodGet, calendarHome(userOne)+"caltest3/")
		require.Equal(t, http.StatusNotFound, res.Status, "body: %s", res.Body)
	})

	t.Run("with incorrect body on existing resource", func(t *testing.T) {
		t.Parallel()

		// Ported from:
		// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/mkcalendar.xml#L155-L172
		t.Skip(
			"RFC 4791 5.3.1: MKCALENDAR on an existing resource must fail with the resource-must-be-null precondition.",
		)

		srv := newTestServer(t)

		res := doRequest(
			t,
			srv,
			webdav.MethodMkcalendar,
			calendarHome(userOne)+"caltest2/",
			withContentType(xmlContentType),
			withBody(loadAppleFixture(t, fixtureMkcalendar1, nil)),
		)
		require.Equal(t, http.StatusCreated, res.Status, "body: %s", res.Body)

		res = doRequest(
			t,
			srv,
			webdav.MethodMkcalendar,
			calendarHome(userOne)+"caltest2/",
			withContentType(xmlContentType),
			withBody(loadAppleFixture(t, "Resource/CalDAV/mkcalendar/2.txt", nil)),
		)
		require.Equal(t, http.StatusPreconditionFailed, res.Status, "body: %s", res.Body)
	})

	t.Run("read-free-busy privilege", func(t *testing.T) {
		t.Parallel()

		// Ported from:
		// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/mkcalendar.xml#L176-L207
		t.Skip("RFC 4791 7.10: free-busy-query is not implemented.")

		srv := newTestServer(t)

		res := createCalendar(t, srv, userOne, calendarHome(userOne)+"caltest3/")
		require.Equal(t, http.StatusCreated, res.Status, "body: %s", res.Body)

		res = report(
			t,
			srv,
			calendarHome(userOne)+"caltest3/",
			loadAppleFixture(t, "Resource/CalDAV/mkcalendar/3.xml", nil),
			asUser(userTwo),
		)
		require.Equal(t, http.StatusOK, res.Status, "body: %s", res.Body)
	})

	t.Run("supported component set", func(t *testing.T) {
		t.Parallel()

		t.Run("vevent only", func(t *testing.T) {
			t.Parallel()

			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/mkcalendar.xml#L211-L242
			// Expected component set comes from restrictions/1.xml and readback data from restrictions/2.ics.
			srv := newTestServer(t)
			path := calendarHome(userOne) + "vevent-only/"

			res := doRequest(
				t,
				srv,
				webdav.MethodMkcalendar,
				path,
				withContentType(xmlContentType),
				withBody(loadAppleFixture(t, "Resource/CalDAV/mkcalendar/restrictions/1.xml", nil)),
			)
			require.Equal(t, http.StatusCreated, res.Status, "body: %s", res.Body)

			prop := calendarProps(t, srv, path)
			assertResourceType(t, prop, true)
			assertSupportedComponents(t, prop, []string{componentVEvent})

			res = putAppleFixture(t, srv, userOne, path+object1, "Resource/CalDAV/mkcalendar/restrictions/2.ics")
			require.Equal(t, http.StatusCreated, res.Status, "body: %s", res.Body)

			res = doRequest(t, srv, http.MethodGet, path+object1)
			requireSuccess(t, res)
			assertCalendarDataEqual(
				t,
				res.Body,
				loadAppleFixture(t, "Resource/CalDAV/mkcalendar/restrictions/2.ics", nil),
			)
		})

		t.Run("vtodo rejected from vevent only", func(t *testing.T) {
			t.Parallel()

			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/mkcalendar.xml#L243-L260
			t.Skip("RFC 4791 5.3.2.1: PUT must reject a component outside supported-calendar-component-set.")

			srv := newTestServer(t)

			res := doRequest(
				t,
				srv,
				webdav.MethodMkcalendar,
				calendarHome(userOne)+"vevent-only/",
				withContentType(xmlContentType),
				withBody(loadAppleFixture(t, "Resource/CalDAV/mkcalendar/restrictions/1.xml", nil)),
			)
			require.Equal(t, http.StatusCreated, res.Status, "body: %s", res.Body)

			res = putAppleFixture(
				t,
				srv,
				userOne,
				calendarHome(userOne)+"vevent-only/2.ics",
				"Resource/CalDAV/mkcalendar/restrictions/3.ics",
			)
			require.Equal(t, http.StatusForbidden, res.Status, "body: %s", res.Body)

			davErr := parseDAVError(t, res.Body)
			assert.True(t, hasName(davErr.Names, caldavNamespace, "supported-calendar-component"))
		})

		t.Run("vtodo only", func(t *testing.T) {
			t.Parallel()

			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/mkcalendar.xml#L261-L278
			// Expected component set comes from restrictions/4.xml and readback data from restrictions/6.ics.
			srv := newTestServer(t)
			path := calendarHome(userOne) + "vtodo-only/"

			res := doRequest(
				t,
				srv,
				webdav.MethodMkcalendar,
				path,
				withContentType(xmlContentType),
				withBody(loadAppleFixture(t, "Resource/CalDAV/mkcalendar/restrictions/4.xml", nil)),
			)
			require.Equal(t, http.StatusCreated, res.Status, "body: %s", res.Body)

			prop := calendarProps(t, srv, path)
			assertResourceType(t, prop, true)
			assertSupportedComponents(t, prop, []string{componentVTodo})

			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/mkcalendar.xml#L297-L310
			res = putAppleFixture(t, srv, userOne, path+object2, "Resource/CalDAV/mkcalendar/restrictions/6.ics")
			require.Equal(t, http.StatusCreated, res.Status, "body: %s", res.Body)

			res = doRequest(t, srv, http.MethodGet, path+object2)
			requireSuccess(t, res)
			assertCalendarDataEqual(
				t,
				res.Body,
				loadAppleFixture(t, "Resource/CalDAV/mkcalendar/restrictions/6.ics", nil),
			)
		})

		t.Run("vevent rejected from vtodo only", func(t *testing.T) {
			t.Parallel()

			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/mkcalendar.xml#L279-L296
			t.Skip("RFC 4791 5.3.2.1: PUT must reject a component outside supported-calendar-component-set.")

			srv := newTestServer(t)

			res := doRequest(
				t,
				srv,
				webdav.MethodMkcalendar,
				calendarHome(userOne)+"vtodo-only/",
				withContentType(xmlContentType),
				withBody(loadAppleFixture(t, "Resource/CalDAV/mkcalendar/restrictions/4.xml", nil)),
			)
			require.Equal(t, http.StatusCreated, res.Status, "body: %s", res.Body)

			res = putAppleFixture(
				t,
				srv,
				userOne,
				calendarHome(userOne)+"vtodo-only/1.ics",
				"Resource/CalDAV/mkcalendar/restrictions/5.ics",
			)
			require.Equal(t, http.StatusForbidden, res.Status, "body: %s", res.Body)

			davErr := parseDAVError(t, res.Body)
			assert.True(t, hasName(davErr.Names, caldavNamespace, "supported-calendar-component"))
		})

		t.Run("vevent and vtodo", func(t *testing.T) {
			t.Parallel()

			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/mkcalendar.xml#L311-L375
			// Expected component set comes from restrictions/7.xml and readback data from 8.ics and 9.ics.
			srv := newTestServer(t)
			path := calendarHome(userOne) + "vevent-vtodo/"

			res := doRequest(
				t,
				srv,
				webdav.MethodMkcalendar,
				path,
				withContentType(xmlContentType),
				withBody(loadAppleFixture(t, "Resource/CalDAV/mkcalendar/restrictions/7.xml", nil)),
			)
			require.Equal(t, http.StatusCreated, res.Status, "body: %s", res.Body)

			prop := calendarProps(t, srv, path)
			assertResourceType(t, prop, true)
			assertSupportedComponents(t, prop, []string{componentVEvent, componentVTodo})

			for _, tt := range []struct {
				name    string
				fixture string
			}{
				{name: object1, fixture: "Resource/CalDAV/mkcalendar/restrictions/8.ics"},
				{name: object2, fixture: "Resource/CalDAV/mkcalendar/restrictions/9.ics"},
			} {
				res = putAppleFixture(t, srv, userOne, path+tt.name, tt.fixture)
				require.Equal(t, http.StatusCreated, res.Status, "body: %s", res.Body)

				res = doRequest(t, srv, http.MethodGet, path+tt.name)
				requireSuccess(t, res)
				assertCalendarDataEqual(t, res.Body, loadAppleFixture(t, tt.fixture, nil))
			}
		})
	})

	t.Run("supported component sets property", func(t *testing.T) {
		t.Parallel()

		// Ported from:
		// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/mkcalendar.xml#L378-L417
		t.Skip("Optional: supported-calendar-component-sets is not implemented.")
	})

	t.Run("single component calendars", func(t *testing.T) {
		t.Parallel()

		// Ported from:
		// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/mkcalendar.xml#L420-L572
		t.Skip(skipSplitCalendars)
	})
}
