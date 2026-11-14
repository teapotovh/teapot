package caldavtest

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAppleDelete(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)

	eventPath := calendarPath(userOne) + "1.ics"
	todoPath := tasksPath(userOne) + "1todo.ics"

	// Ported from:
	// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/delete.xml#L29-L46
	event := loadAppleFixture(t, "Resource/CalDAV/delete/1.txt", nil)
	res := putObject(t, srv, userOne, eventPath, event)
	require.Equal(t, http.StatusCreated, res.Status, "body: %s", res.Body)

	todo := loadAppleFixture(t, "Resource/CalDAV/todo/1.txt", nil)
	res = putObject(t, srv, userOne, todoPath, todo)
	require.Equal(t, http.StatusCreated, res.Status, "body: %s", res.Body)

	// Ported from:
	// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/delete.xml#L49-L58
	t.Run("delete event", func(t *testing.T) {
		t.Parallel()

		res := deleteObject(t, srv, userOne, eventPath)
		require.GreaterOrEqual(t, res.Status, 200)
		require.Less(t, res.Status, 300)
	})

	// Ported from:
	// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/delete.xml#L59-L68
	t.Run("delete todo", func(t *testing.T) {
		t.Parallel()

		res := deleteObject(t, srv, userOne, todoPath)
		require.GreaterOrEqual(t, res.Status, 200)
		require.Less(t, res.Status, 300)
	})

	// Ported from:
	// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/delete.xml#L69-L82
	t.Run("delete missing object", func(t *testing.T) {
		t.Parallel()

		res := deleteObject(t, srv, userOne, calendarPath(userOne)+"2.ics")
		require.Equal(t, http.StatusNotFound, res.Status, "body: %s", res.Body)
	})
}
