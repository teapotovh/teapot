//nolint:paralleltest // the integration tests share one database.
package integration

import (
	"context"
	"encoding/xml"
	"log/slog"
	"os"
	"testing"
	"time"
	"uuid"

	postgres "github.com/fergusstrange/embedded-postgres/v2"
	"github.com/fergusstrange/embedded-postgres/v2/eptest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/teapotovh/teapot/service/calendar/store"
)

var (
	testPSQLURL string
	testOnline  *store.Online
	errTestPSQL error
)

func TestMain(m *testing.M) {
	code := eptest.Run(m, func(pg *postgres.EmbeddedPostgres) {
		testPSQLURL = pg.ConnectionURL()
		testOnline, errTestPSQL = store.NewOnline(
			context.Background(),
			testPSQLURL,
			store.StoreS3Config{},
			slog.New(slog.DiscardHandler),
		)
	})
	os.Exit(code)
}

func newIntegrationOnline(t *testing.T) *store.Online {
	t.Helper()

	require.NoError(t, errTestPSQL)
	require.NotNil(t, testOnline)

	conn := testPostgresConn(t)

	if _, err := conn.Exec(t.Context(), "TRUNCATE calendars, object_refs"); err != nil {
		t.Fatalf("truncate tables: %v", err)
	}

	return testOnline
}

func testPostgresConn(t *testing.T) *pgx.Conn {
	t.Helper()

	conn, err := pgx.Connect(t.Context(), testPSQLURL)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, conn.Close(context.Background()))
	})

	return conn
}

func testCalendar(path store.Path) store.Calendar {
	return store.Calendar{
		Path: path,
		Metadata: store.CalendarMetadata{
			Name:                  "Test calendar",
			Description:           "A calendar used in tests",
			SupportedComponentSet: []string{"VEVENT", "VTODO"},
			MaxResourceSize:       1024,
			Color:                 "#ff0000",
			Tag:                   "test",
			DeadProperties: store.DeadProperties{
				xml.Name{Space: "DAV:", Local: "details"}: "<details xmlns=\"DAV:\">My Name</details>",
			},
		},
	}
}

func putCalendar(t *testing.T, o *store.Online, calendar store.Calendar) bool {
	t.Helper()

	created, err := o.PutCalendar(t.Context(), calendar)
	require.NoError(t, err)

	return created
}

func TestOnlineIntegrationCalendarJSONRoundTrip(t *testing.T) {
	o := newIntegrationOnline(t)
	calendar := testCalendar("/user/calendars/test")

	assert.True(t, putCalendar(t, o, calendar))

	got, err := o.GetCalendar(t.Context(), calendar.Path)
	require.NoError(t, err)
	assert.Equal(t, calendar, *got)
}

func TestOnlineIntegrationCalendarNotFound(t *testing.T) {
	o := newIntegrationOnline(t)

	_, err := o.GetCalendar(t.Context(), "/user/calendars/missing")
	require.ErrorIs(t, err, store.ErrCalendarNotFound)
}

func TestOnlineIntegrationCalendarPrefixOrdering(t *testing.T) {
	o := newIntegrationOnline(t)

	calendars := []store.Calendar{
		testCalendar("/user/calendars/b"),
		testCalendar("/user/calendars/a/event"),
		testCalendar("/other"),
		testCalendar("/user/calendars/a"),
	}

	for _, calendar := range calendars {
		putCalendar(t, o, calendar)
	}

	got, err := o.ListCalendars(t.Context(), "")
	require.NoError(t, err)
	require.Len(t, got, len(calendars))

	assert.Equal(t, store.Path("/other"), got[0].Path)
	assert.Equal(t, store.Path("/user/calendars/a"), got[1].Path)
	assert.Equal(t, store.Path("/user/calendars/a/event"), got[2].Path)
	assert.Equal(t, store.Path("/user/calendars/b"), got[3].Path)

	got, err = o.ListCalendars(t.Context(), "/user/calendars/a")
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, store.Path("/user/calendars/a"), got[0].Path)
	assert.Equal(t, store.Path("/user/calendars/a/event"), got[1].Path)
}

func TestOnlineIntegrationCalendarUpdate(t *testing.T) {
	o := newIntegrationOnline(t)
	calendar := testCalendar("/user/calendars/test")

	assert.True(t, putCalendar(t, o, calendar))

	calendar.Metadata.Name = "Updated calendar"
	assert.False(t, putCalendar(t, o, calendar))

	got, err := o.GetCalendar(t.Context(), calendar.Path)
	require.NoError(t, err)
	assert.Equal(t, "Updated calendar", got.Metadata.Name)
}

func TestOnlineIntegrationCalendarCommitRollback(t *testing.T) {
	o := newIntegrationOnline(t)
	committed := testCalendar("/user/calendars/commit")
	discarded := testCalendar("/user/calendars/rollback")

	tx, err := o.Begin(t.Context())
	require.NoError(t, err)
	require.NoError(t, tx.StoreCalendar(t.Context(), committed))
	require.NoError(t, tx.Commit(t.Context()))

	_, err = o.GetCalendar(t.Context(), committed.Path)
	require.NoError(t, err)

	tx, err = o.Begin(t.Context())
	require.NoError(t, err)
	require.NoError(t, tx.StoreCalendar(t.Context(), discarded))
	require.NoError(t, tx.Rollback(t.Context()))

	_, err = o.GetCalendar(t.Context(), discarded.Path)
	require.ErrorIs(t, err, store.ErrCalendarNotFound)
}

func TestOnlineIntegrationObjectRefLifecycle(t *testing.T) {
	o := newIntegrationOnline(t)
	conn := testPostgresConn(t)
	path := store.Path("/user/calendars/test/event.ics")
	modTime := time.Date(2026, time.January, 15, 10, 30, 0, 0, time.UTC)
	ref := uuid.MustParse("11111111-1111-1111-1111-111111111111")

	tx, err := o.Begin(t.Context())
	require.NoError(t, err)
	require.NoError(t, tx.StoreObjectRef(t.Context(), store.ObjectRef{
		Path:    path,
		ModTime: modTime,
		Ref:     ref,
		ETag:    "etag",
	}))
	require.NoError(t, tx.Commit(t.Context()))

	got := readObjectRef(t, conn, path)
	assert.Equal(t, modTime, got.ModTime.UTC())
	assert.Equal(t, ref, got.Ref)
	assert.Equal(t, "etag", got.ETag.String())

	updatedModTime := modTime.Add(time.Hour)
	tx, err = o.Begin(t.Context())
	require.NoError(t, err)
	require.NoError(t, tx.StoreObjectRef(t.Context(), store.ObjectRef{
		Path:    path,
		ModTime: updatedModTime,
		Ref:     ref,
		ETag:    "updated-etag",
	}))
	require.NoError(t, tx.Commit(t.Context()))

	got = readObjectRef(t, conn, path)
	assert.Equal(t, updatedModTime, got.ModTime.UTC())
	assert.Equal(t, "updated-etag", got.ETag.String())

	tx, err = o.Begin(t.Context())
	require.NoError(t, err)
	require.NoError(t, tx.DeleteObjectRef(t.Context(), path))
	require.NoError(t, tx.Commit(t.Context()))

	var count int
	require.NoError(t, conn.QueryRow(t.Context(), "SELECT COUNT(*) FROM object_refs").Scan(&count))
	assert.Zero(t, count)
}

func TestOnlineIntegrationObjectRefRollback(t *testing.T) {
	o := newIntegrationOnline(t)
	conn := testPostgresConn(t)
	path := store.Path("/user/calendars/test/event.ics")

	tx, err := o.Begin(t.Context())
	require.NoError(t, err)
	require.NoError(t, tx.StoreObjectRef(t.Context(), store.ObjectRef{
		Path:    path,
		ModTime: time.Date(2026, time.January, 15, 10, 30, 0, 0, time.UTC),
		Ref:     uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		ETag:    "etag",
	}))
	require.NoError(t, tx.Rollback(t.Context()))

	var count int
	require.NoError(t, conn.QueryRow(t.Context(), "SELECT COUNT(*) FROM object_refs").Scan(&count))
	assert.Zero(t, count)
}

func TestOnlineIntegrationObjectRefPrefixOrdering(t *testing.T) {
	o := newIntegrationOnline(t)
	conn := testPostgresConn(t)
	paths := []store.Path{
		"/user/calendars/test/b.ics",
		"/user/calendars/test/a.ics",
		"/user/calendars/other/c.ics",
	}

	tx, err := o.Begin(t.Context())
	require.NoError(t, err)

	for i, path := range paths {
		require.NoError(t, tx.StoreObjectRef(t.Context(), store.ObjectRef{
			Path:    path,
			ModTime: time.Date(2026, time.January, 15+i, 10, 30, 0, 0, time.UTC),
			Ref:     uuid.New(),
			ETag:    "etag",
		}))
	}

	require.NoError(t, tx.Commit(t.Context()))

	rows, err := conn.Query(t.Context(), "SELECT path FROM object_refs ORDER BY path")
	require.NoError(t, err)

	defer rows.Close()

	var got []store.Path

	for rows.Next() {
		var path string
		require.NoError(t, rows.Scan(&path))
		got = append(got, store.Path(path))
	}

	require.NoError(t, rows.Err())
	assert.Equal(t, []store.Path{
		"/user/calendars/other/c.ics",
		"/user/calendars/test/a.ics",
		"/user/calendars/test/b.ics",
	}, got)
}

func TestOnlineIntegrationPathCollation(t *testing.T) {
	newIntegrationOnline(t)
	conn := testPostgresConn(t)

	for _, table := range []string{"calendars", "object_refs"} {
		var collation string

		err := conn.QueryRow(
			t.Context(),
			`SELECT c.collname
			FROM pg_attribute a
			JOIN pg_collation c ON c.oid = a.attcollation
			WHERE a.attrelid = $1::regclass
				AND a.attname = 'path';`,
			table,
		).Scan(&collation)
		require.NoError(t, err)
		assert.Equal(t, "C", collation)
	}
}

func readObjectRef(t *testing.T, conn *pgx.Conn, path store.Path) store.ObjectRef {
	t.Helper()

	var (
		ref   store.ObjectRef
		refID pgtype.UUID
	)

	err := conn.QueryRow(
		t.Context(),
		"SELECT path, mod_time, ref, etag FROM object_refs WHERE path = $1",
		path.String(),
	).Scan(&ref.Path, &ref.ModTime, &refID, &ref.ETag)
	require.NoError(t, err)

	ref.Ref = uuid.UUID(refID.Bytes)

	return ref
}
