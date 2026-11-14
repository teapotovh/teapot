package caldavtest

import (
	"encoding/xml"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const resourceTypeQuery = `<D:propfind xmlns:D="DAV:"><D:prop><D:resourcetype/></D:prop></D:propfind>`

func TestAppleConditional(t *testing.T) {
	t.Parallel()

	t.Run("regular propfind", func(t *testing.T) {
		t.Parallel()

		// Ported from:
		// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/conditional.xml#L32-L57
		srv := newTestServer(t)

		res := propFind(
			t,
			srv,
			calendarPath(userOne),
			withDepth("0"),
			withBody(loadAppleFixture(t, fixtureDisplayName, nil)),
		)
		require.Equal(t, http.StatusMultiStatus, res.Status, "body: %s", res.Body)
	})

	t.Run("conditional propfind", func(t *testing.T) {
		t.Parallel()

		// Ported from:
		// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/conditional.xml#L67-L92
		t.Skip(skipConditionalPropfind)

		srv := newTestServer(t)

		res := propFind(
			t,
			srv,
			calendarPath(userOne),
			withDepth("0"),
			withBody(loadAppleFixture(t, fixtureDisplayName, nil)),
		)
		require.Equal(t, http.StatusMultiStatus, res.Status, "body: %s", res.Body)

		res = propFind(
			t,
			srv,
			calendarPath(userOne),
			withDepth("0"),
			withHeader("If-Modified-Since", res.Header.Get("Last-Modified")),
			withBody(loadAppleFixture(t, fixtureDisplayName, nil)),
		)
		require.Equal(t, http.StatusNotModified, res.Status, "body: %s", res.Body)
	})

	t.Run("if-modified-since is ignored", func(t *testing.T) {
		t.Parallel()

		// RFC 9110, section 13.1.3: https://www.rfc-editor.org/rfc/rfc9110#section-13.1.3
		srv := newTestServer(t)

		res := propFind(
			t,
			srv,
			calendarPath(userOne),
			withDepth("0"),
			withHeader("If-Modified-Since", time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat)),
			withBody(resourceTypeQuery),
		)
		require.Equal(t, http.StatusMultiStatus, res.Status, "body: %s", res.Body)

		prop := assertPropstat(t, res.Body, calendarPath(userOne), http.StatusOK)
		assertResourceType(t, prop, true)
		assertPropNames(
			t,
			res.Body,
			calendarPath(userOne),
			http.StatusOK,
			xml.Name{Space: davNamespace, Local: "resourcetype"},
		)
	})
}
