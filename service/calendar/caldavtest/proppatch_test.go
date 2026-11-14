package caldavtest

import (
	"encoding/xml"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/teapotovh/teapot/lib/webdav"
)

var (
	detailsProperty  = xml.Name{Space: davNamespace, Local: "details"}
	details2Property = xml.Name{Space: davNamespace, Local: "details2"}
	missingProperty  = xml.Name{Space: davNamespace, Local: "missing"}
)

func TestAppleProppatch(t *testing.T) {
	t.Parallel()

	t.Run("set details", func(t *testing.T) {
		t.Parallel()

		// Ported from:
		// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/proppatch.xml#L29-L67
		t.Skip(skipProppatch)

		srv := newTestServer(t)

		res := doRequest(
			t,
			srv,
			webdav.MethodPropPatch,
			calendarPath(userOne),
			withContentType(xmlContentType),
			withBody(loadAppleFixture(t, "Resource/CalDAV/proppatches/1.xml", nil)),
		)
		require.Equal(t, http.StatusMultiStatus, res.Status, "body: %s", res.Body)
		assertPropNames(t, res.Body, calendarPath(userOne), http.StatusOK, detailsProperty, details2Property)

		res = propFind(
			t,
			srv,
			calendarPath(userOne),
			withDepth("0"),
			withBody(loadAppleFixture(t, "Resource/CalDAV/proppatches/2.xml", nil)),
		)
		require.Equal(t, http.StatusMultiStatus, res.Status, "body: %s", res.Body)
		assertPropValue(t, res.Body, calendarPath(userOne), http.StatusOK, detailsProperty, "My Name")
		assertPropValue(t, res.Body, calendarPath(userOne), http.StatusOK, details2Property, "My Name 2")
	})

	t.Run("remove details", func(t *testing.T) {
		t.Parallel()

		// Ported from:
		// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/proppatch.xml#L68-L108
		t.Skip(skipProppatch)

		srv := newTestServer(t)

		res := doRequest(
			t,
			srv,
			webdav.MethodPropPatch,
			calendarPath(userOne),
			withContentType(xmlContentType),
			withBody(loadAppleFixture(t, "Resource/CalDAV/proppatches/3.xml", nil)),
		)
		require.Equal(t, http.StatusMultiStatus, res.Status, "body: %s", res.Body)

		res = propFind(
			t,
			srv,
			calendarPath(userOne),
			withDepth("0"),
			withBody(loadAppleFixture(t, "Resource/CalDAV/proppatches/2.xml", nil)),
		)
		require.Equal(t, http.StatusMultiStatus, res.Status, "body: %s", res.Body)
		assertPropValue(t, res.Body, calendarPath(userOne), http.StatusOK, details2Property, "My Name 2")
		assertPropAbsent(t, res.Body, calendarPath(userOne), detailsProperty)
	})

	t.Run("remove missing property", func(t *testing.T) {
		t.Parallel()

		// Ported from:
		// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/proppatch.xml#L109-L126
		t.Skip(skipProppatch)

		srv := newTestServer(t)

		res := doRequest(
			t,
			srv,
			webdav.MethodPropPatch,
			calendarPath(userOne),
			withContentType(xmlContentType),
			withBody(loadAppleFixture(t, "Resource/CalDAV/proppatches/4.xml", nil)),
		)
		require.Equal(t, http.StatusMultiStatus, res.Status, "body: %s", res.Body)
		assertPropNames(t, res.Body, calendarPath(userOne), http.StatusOK, missingProperty)
	})

	t.Run("change resourcetype", func(t *testing.T) {
		t.Parallel()

		// Ported from:
		// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/proppatch.xml#L127-L169
		t.Skip(skipProppatch)

		srv := newTestServer(t)

		res := doRequest(
			t,
			srv,
			webdav.MethodPropPatch,
			calendarPath(userOne),
			withContentType(xmlContentType),
			withBody(loadAppleFixture(t, "Resource/CalDAV/proppatches/5.xml", nil)),
		)
		require.Equal(t, http.StatusMultiStatus, res.Status, "body: %s", res.Body)
		assertPropNames(
			t,
			res.Body,
			calendarPath(userOne),
			http.StatusForbidden,
			detailsProperty,
			details2Property,
			xml.Name{Space: davNamespace, Local: "resourcetype"},
		)

		res = propFind(
			t,
			srv,
			calendarPath(userOne),
			withDepth("0"),
			withBody(loadAppleFixture(t, "Resource/CalDAV/proppatches/2.xml", nil)),
		)
		require.Equal(t, http.StatusMultiStatus, res.Status, "body: %s", res.Body)
		assertPropValue(t, res.Body, calendarPath(userOne), http.StatusOK, details2Property, "My Name 2")
		assertPropAbsent(t, res.Body, calendarPath(userOne), detailsProperty)
	})

	t.Run("encoded value", func(t *testing.T) {
		t.Parallel()

		// Ported from:
		// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/proppatch.xml#L170-L206
		t.Skip(skipProppatch)

		srv := newTestServer(t)

		res := doRequest(
			t,
			srv,
			webdav.MethodPropPatch,
			calendarPath(userOne),
			withContentType(xmlContentType),
			withBody(loadAppleFixture(t, "Resource/CalDAV/proppatches/8.xml", nil)),
		)
		require.Equal(t, http.StatusMultiStatus, res.Status, "body: %s", res.Body)

		res = propFind(
			t,
			srv,
			calendarPath(userOne),
			withDepth("0"),
			withBody(loadAppleFixture(t, "Resource/CalDAV/proppatches/9.xml", nil)),
		)
		require.Equal(t, http.StatusMultiStatus, res.Status, "body: %s", res.Body)
		assertPropValue(
			t,
			res.Body,
			calendarPath(userOne),
			http.StatusOK,
			xml.Name{Space: davNamespace, Local: "displayname"},
			"calendar <- & ]]>",
		)
	})

	t.Run("missing calendar", func(t *testing.T) {
		t.Parallel()

		// Ported from:
		// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/proppatch.xml#L207-L224
		t.Skip(skipProppatch)

		srv := newTestServer(t)

		res := doRequest(
			t,
			srv,
			webdav.MethodPropPatch,
			calendarHome(userOne)+"bogus-calendar/",
			withContentType(xmlContentType),
			withBody(loadAppleFixture(t, "Resource/CalDAV/proppatches/3.xml", nil)),
		)
		require.Equal(t, http.StatusNotFound, res.Status, "body: %s", res.Body)
	})

	t.Run("property attribute", func(t *testing.T) {
		t.Parallel()

		// Ported from:
		// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/proppatch.xml#L228-L271
		t.Skip(skipProppatch)

		srv := newTestServer(t)

		res := doRequest(
			t,
			srv,
			webdav.MethodPropPatch,
			calendarPath(userOne),
			withContentType(xmlContentType),
			withBody(loadAppleFixture(t, "Resource/CalDAV/proppatches/6.xml", nil)),
		)
		require.Equal(t, http.StatusMultiStatus, res.Status, "body: %s", res.Body)

		res = propFind(
			t,
			srv,
			calendarPath(userOne),
			withDepth("0"),
			withBody(loadAppleFixture(t, "Resource/CalDAV/proppatches/7.xml", nil)),
		)
		require.Equal(t, http.StatusMultiStatus, res.Status, "body: %s", res.Body)
		assertPropValue(t, res.Body, calendarPath(userOne), http.StatusOK, detailsProperty, "My Name")
		assert.Contains(t, res.Body, "details xml:lang='en'")
	})

	t.Run("remove property with attribute", func(t *testing.T) {
		t.Parallel()

		// Ported from:
		// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/proppatch.xml#L272-L312
		t.Skip(skipProppatch)

		srv := newTestServer(t)

		res := doRequest(
			t,
			srv,
			webdav.MethodPropPatch,
			calendarPath(userOne),
			withContentType(xmlContentType),
			withBody(loadAppleFixture(t, "Resource/CalDAV/proppatches/3.xml", nil)),
		)
		require.Equal(t, http.StatusMultiStatus, res.Status, "body: %s", res.Body)

		res = propFind(
			t,
			srv,
			calendarPath(userOne),
			withDepth("0"),
			withBody(loadAppleFixture(t, "Resource/CalDAV/proppatches/2.xml", nil)),
		)
		require.Equal(t, http.StatusMultiStatus, res.Status, "body: %s", res.Body)
		assertPropValue(t, res.Body, calendarPath(userOne), http.StatusOK, details2Property, "My Name 2")
		assertPropAbsent(t, res.Body, calendarPath(userOne), detailsProperty)
	})
}
