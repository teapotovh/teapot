package caldavtest

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppleGet(t *testing.T) {
	t.Parallel()

	t.Run("resource in calendar", func(t *testing.T) {
		t.Parallel()

		// Ported from:
		// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/get.xml#L50-L67
		// Expected data comes from Resource/CalDAV/delete/1.txt.
		srv := newTestServer(t)

		res := putAppleFixture(t, srv, userOne, calendarPath(userOne)+object1, fixtureDelete1)
		requireSuccess(t, res)

		res = doRequest(t, srv, http.MethodGet, calendarPath(userOne)+object1)
		require.Equal(t, http.StatusOK, res.Status, "body: %s", res.Body)
		assert.NotEmpty(t, res.Header.Get("ETag"))
		assert.NotEmpty(t, res.Header.Get("Last-Modified"))
		assertCalendarDataEqual(t, res.Body, loadAppleFixture(t, fixtureDelete1, nil))
	})

	t.Run("resource in regular collection", func(t *testing.T) {
		t.Parallel()

		// Ported from:
		// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/get.xml#L68-L87
		t.Skip("Optional: regular-collection is not planned.")

		srv := newTestServer(t)

		res := putAppleFixture(t, srv, userOne, calendarHome(userOne)+"1todo.ics", "Resource/CalDAV/todo/1.txt")
		requireSuccess(t, res)

		res = doRequest(t, srv, http.MethodGet, calendarHome(userOne)+"1todo.ics")
		require.Equal(t, http.StatusOK, res.Status, "body: %s", res.Body)
		assertCalendarDataEqual(t, res.Body, loadAppleFixture(t, "Resource/CalDAV/todo/1.txt", nil))
	})

	t.Run("calendar collection", func(t *testing.T) {
		t.Parallel()

		// Ported from:
		// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/get.xml#L90-L398
		t.Skip(skipCollectionListing)

		srv := newTestServer(t)

		res := putAppleFixture(t, srv, userOne, calendarPath(userOne)+object1, fixtureDelete1)
		requireSuccess(t, res)

		res = doRequest(
			t,
			srv,
			http.MethodGet,
			calendarPath(userOne),
			withHeader("User-Agent", "Mozilla/Gecko"),
		)
		require.Equal(t, http.StatusOK, res.Status, "body: %s", res.Body)
		assert.Contains(t, res.Body, "<td>text/calendar</td>")

		res = doRequest(t, srv, http.MethodGet, calendarPath(userOne))
		require.Equal(t, http.StatusOK, res.Status, "body: %s", res.Body)
		assert.Contains(t, res.Body, "BEGIN:VEVENT")
		assert.NotEmpty(t, res.Header.Get("ETag"))
		assert.NotEmpty(t, res.Header.Get("Last-Modified"))
	})

	t.Run("calendar home", func(t *testing.T) {
		t.Parallel()

		// Ported from:
		// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/get.xml#L400-L538
		t.Skip(skipCollectionListing)

		srv := newTestServer(t)

		res := doRequest(t, srv, http.MethodGet, calendarHome(userOne))
		require.Equal(t, http.StatusOK, res.Status, "body: %s", res.Body)
		etag := res.Header.Get("ETag")
		lastModified := res.Header.Get("Last-Modified")

		assert.NotEmpty(t, etag)
		assert.NotEmpty(t, lastModified)

		res = createCalendar(t, srv, userOne, calendarHome(userOne)+"testcal/")
		require.Equal(t, http.StatusCreated, res.Status, "body: %s", res.Body)

		res = doRequest(t, srv, http.MethodGet, calendarHome(userOne))
		require.Equal(t, http.StatusOK, res.Status, "body: %s", res.Body)
		assert.NotEqual(t, etag, res.Header.Get("ETag"))
		assert.NotEqual(t, lastModified, res.Header.Get("Last-Modified"))

		res = doRequest(t, srv, http.MethodGet, calendarHome(userOne), withHeader("If-None-Match", etag))
		require.Equal(t, http.StatusNotModified, res.Status, "body: %s", res.Body)

		res = deleteObject(t, srv, userOne, calendarHome(userOne)+"testcal/")
		requireSuccess(t, res)
	})
}
