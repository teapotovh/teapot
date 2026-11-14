package caldavtest

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testEvent = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Teapot//Test//EN
BEGIN:VEVENT
UID:test-event@example.com
DTSTAMP:20260101T000000Z
DTSTART:20260101T120000Z
DTEND:20260101T130000Z
SUMMARY:Test event
END:VEVENT
END:VCALENDAR`

	updatedTestEvent = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Teapot//Test//EN
BEGIN:VEVENT
UID:test-event@example.com
DTSTAMP:20260101T000000Z
DTSTART:20260101T120000Z
DTEND:20260101T130000Z
SUMMARY:Updated test event
END:VEVENT
END:VCALENDAR`
)

func TestDeleteObjectPreconditions(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)

	path := calendarPath(userOne) + "delete-preconditions.ics"

	res := putObject(t, srv, userOne, path, testEvent)
	require.Equal(t, http.StatusCreated, res.Status, "body: %s", res.Body)

	res = doRequest(t, srv, http.MethodGet, path, asUser(userOne))
	require.Equal(t, http.StatusOK, res.Status, "body: %s", res.Body)

	etag := res.Header.Get("ETag")
	require.NotEmpty(t, etag)

	res = doRequest(
		t,
		srv,
		http.MethodDelete,
		path,
		asUser(userOne),
		withHeader("If-Match", `"missing"`),
	)
	require.Equal(t, http.StatusPreconditionFailed, res.Status, "body: %s", res.Body)

	res = doRequest(t, srv, http.MethodGet, path, asUser(userOne))
	require.Equal(t, http.StatusOK, res.Status, "body: %s", res.Body)

	res = doRequest(
		t,
		srv,
		http.MethodDelete,
		path,
		asUser(userOne),
		withHeader("If-Match", etag),
	)
	require.Equal(t, http.StatusNoContent, res.Status, "body: %s", res.Body)

	res = doRequest(t, srv, http.MethodGet, path, asUser(userOne))
	require.Equal(t, http.StatusNotFound, res.Status, "body: %s", res.Body)
}

func TestPutObjectPreconditions(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)

	path := calendarPath(userOne) + "preconditions.ics"

	res := putObject(t, srv, userOne, path, testEvent)
	require.Equal(t, http.StatusCreated, res.Status, "body: %s", res.Body)

	res = doRequest(t, srv, http.MethodGet, path, asUser(userOne))
	require.Equal(t, http.StatusOK, res.Status, "body: %s", res.Body)

	etag := res.Header.Get("ETag")
	require.NotEmpty(t, etag)

	res = doRequest(
		t,
		srv,
		http.MethodPut,
		path,
		asUser(userOne),
		withContentType(calendarContentType),
		withBody(updatedTestEvent),
		withHeader("If-Match", etag),
	)
	require.Equal(t, http.StatusNoContent, res.Status, "body: %s", res.Body)

	res = doRequest(
		t,
		srv,
		http.MethodPut,
		path,
		asUser(userOne),
		withContentType(calendarContentType),
		withBody(testEvent),
		withHeader("If-Match", etag),
	)
	require.Equal(t, http.StatusPreconditionFailed, res.Status, "body: %s", res.Body)

	res = doRequest(
		t,
		srv,
		http.MethodPut,
		path,
		asUser(userOne),
		withContentType(calendarContentType),
		withBody(testEvent),
		withHeader("If-None-Match", "*"),
	)
	require.Equal(t, http.StatusPreconditionFailed, res.Status, "body: %s", res.Body)

	res = doRequest(t, srv, http.MethodGet, path, asUser(userOne))
	require.Equal(t, http.StatusOK, res.Status, "body: %s", res.Body)
	assert.Contains(t, res.Body, "SUMMARY:Updated test event")
}
