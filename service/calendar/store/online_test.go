package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/teapotovh/teapot/lib/s3cache"
)

var (
	errOnlineTest   = errors.New("psql test error")
	errBlobNotFound = errors.New("blob not found")
)

const (
	testColumnPath     = "path"
	testColumnMetadata = "metadata"
	testColumnModTime  = "mod_time"
	testColumnRef      = "ref"
	testColumnETag     = "etag"
)

type memBlob struct {
	data map[string][]byte
}

func newMemBlob() *memBlob {
	return &memBlob{data: map[string][]byte{}}
}

func (b *memBlob) Put(_ context.Context, key string, r io.Reader) (s3cache.Hash, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return s3cache.ZeroHash, err
	}

	b.data[key] = data

	return s3cache.HashBytes(data), nil
}

func (b *memBlob) Get(_ context.Context, key string, _ s3cache.Hash) (io.ReadSeekCloser, s3cache.Hash, error) {
	data, ok := b.data[key]
	if !ok {
		return nil, s3cache.ZeroHash, errBlobNotFound
	}

	return &seekCloser{Reader: bytes.NewReader(data)}, s3cache.HashBytes(data), nil
}

func (b *memBlob) Remove(_ context.Context, key string) error {
	delete(b.data, key)
	return nil
}

type seekCloser struct {
	*bytes.Reader
}

func (s *seekCloser) Close() error {
	return nil
}

func newMockOnline(t *testing.T) (*Online, pgxmock.PgxPoolIface) {
	t.Helper()

	pool, err := pgxmock.NewPool(pgxmock.QueryMatcherOption(pgxmock.QueryMatcherEqual))
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	o := Online{pool: pool}
	o.metrics.initMetrics("test")

	return &o, pool
}

func testCalendarRows(t *testing.T, path Path, metadata CalendarMetadata) *pgxmock.Rows {
	t.Helper()

	rawMetadata, err := json.Marshal(metadata)
	require.NoError(t, err)

	return pgxmock.NewRows([]string{testColumnPath, testColumnMetadata}).AddRow(path.String(), rawMetadata)
}

func testObjectRefRows(path Path, modTime time.Time, ref uuid.UUID, etag s3cache.Hash) *pgxmock.Rows {
	return pgxmock.NewRows([]string{testColumnPath, testColumnModTime, testColumnRef, testColumnETag}).
		AddRow(path.String(), modTime, pgtype.UUID{Bytes: ref, Valid: true}, etag.String())
}

func beginTestTransaction(t *testing.T, o *Online, pool pgxmock.PgxPoolIface) Transaction {
	t.Helper()

	pool.ExpectBegin()

	tx, err := o.Begin(t.Context())
	require.NoError(t, err)

	return tx
}

func TestOnlineGetCalendar(t *testing.T) {
	t.Parallel()

	o, pool := newMockOnline(t)
	path := Path("/user/calendars/test")
	metadata := CalendarMetadata{Name: "Test", Color: "#ff0000"}

	pool.ExpectQuery(getCalendarQuery).
		WithArgs(path.String()).
		WillReturnRows(testCalendarRows(t, path, metadata))

	calendar, err := o.GetCalendar(t.Context(), path)
	require.NoError(t, err)
	assert.Equal(t, path, calendar.Path)
	assert.Equal(t, metadata, calendar.Metadata)
	require.NoError(t, pool.ExpectationsWereMet())
}

func TestOnlineGetCalendarNotFound(t *testing.T) {
	t.Parallel()

	o, pool := newMockOnline(t)
	path := Path("/user/calendars/missing")

	pool.ExpectQuery(getCalendarQuery).
		WithArgs(path.String()).
		WillReturnRows(pgxmock.NewRows([]string{testColumnPath, testColumnMetadata}))

	_, err := o.GetCalendar(t.Context(), path)
	require.ErrorIs(t, err, ErrCalendarNotFound)
	require.NoError(t, pool.ExpectationsWereMet())
}

func TestOnlineGetCalendarQueryError(t *testing.T) {
	t.Parallel()

	o, pool := newMockOnline(t)
	path := Path("/user/calendars/test")

	pool.ExpectQuery(getCalendarQuery).
		WithArgs(path.String()).
		WillReturnError(errOnlineTest)

	_, err := o.GetCalendar(t.Context(), path)
	require.ErrorIs(t, err, errOnlineTest)
	require.ErrorContains(t, err, "error while getting calendar from psql")
	require.NoError(t, pool.ExpectationsWereMet())
}

func TestOnlineGetCalendarDecodeError(t *testing.T) {
	t.Parallel()

	o, pool := newMockOnline(t)
	path := Path("/user/calendars/test")

	pool.ExpectQuery(getCalendarQuery).
		WithArgs(path.String()).
		WillReturnRows(pgxmock.NewRows([]string{testColumnPath, testColumnMetadata}).AddRow(path.String(), []byte(`{`)))

	_, err := o.GetCalendar(t.Context(), path)
	require.ErrorContains(t, err, "error while decoding JSON calendar metadata field")
	require.NoError(t, pool.ExpectationsWereMet())
}

func TestOnlineListCalendarsAll(t *testing.T) {
	t.Parallel()

	o, pool := newMockOnline(t)
	first := Path("/user/calendars/a")
	second := Path("/user/calendars/b")

	pool.ExpectQuery(listCalendarQuery).WillReturnRows(
		pgxmock.NewRows([]string{testColumnPath, testColumnMetadata}).
			AddRow(first.String(), []byte(`{"name":"A"}`)).
			AddRow(second.String(), []byte(`{"name":"B"}`)),
	)

	calendars, err := o.ListCalendars(t.Context(), "")
	require.NoError(t, err)
	require.Len(t, calendars, 2)
	assert.Equal(t, first, calendars[0].Path)
	assert.Equal(t, second, calendars[1].Path)
	require.NoError(t, pool.ExpectationsWereMet())
}

func TestOnlineListCalendarsRange(t *testing.T) {
	t.Parallel()

	o, pool := newMockOnline(t)
	base := Path("/user/calendars")
	path := Path("/user/calendars/test")

	pool.ExpectQuery(rangeCalendarQuery).
		WithArgs(base.String(), prefixEnd(base).String()).
		WillReturnRows(pgxmock.NewRows([]string{testColumnPath, testColumnMetadata}).
			AddRow(path.String(), []byte(`{"name":"Test"}`)))

	calendars, err := o.ListCalendars(t.Context(), base)
	require.NoError(t, err)
	require.Len(t, calendars, 1)
	assert.Equal(t, path, calendars[0].Path)
	require.NoError(t, pool.ExpectationsWereMet())
}

func TestOnlineListCalendarsError(t *testing.T) {
	t.Parallel()

	o, pool := newMockOnline(t)
	base := Path("/user/calendars")

	pool.ExpectQuery(rangeCalendarQuery).
		WithArgs(base.String(), prefixEnd(base).String()).
		WillReturnError(errOnlineTest)

	_, err := o.ListCalendars(t.Context(), base)
	require.ErrorIs(t, err, errOnlineTest)
	require.ErrorContains(t, err, "error while listing calendars from psql")
	require.NoError(t, pool.ExpectationsWereMet())
}

func TestOnlinePutCalendar(t *testing.T) {
	t.Parallel()

	o, pool := newMockOnline(t)
	calendar := Calendar{
		Path:     "/user/calendars/test",
		Metadata: CalendarMetadata{Name: "Test"},
	}
	rawMetadata, err := json.Marshal(calendar.Metadata)
	require.NoError(t, err)

	pool.ExpectQuery(getCalendarQuery).
		WithArgs(calendar.Path.String()).
		WillReturnRows(pgxmock.NewRows([]string{testColumnPath, testColumnMetadata}))
	pool.ExpectBegin()
	pool.ExpectExec(storeCalendarQuery).
		WithArgs(calendar.Path.String(), rawMetadata).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	pool.ExpectCommit()

	created, err := o.PutCalendar(t.Context(), calendar)
	require.NoError(t, err)
	assert.True(t, created)
	require.NoError(t, pool.ExpectationsWereMet())
}

func TestOnlinePutCalendarUpdate(t *testing.T) {
	t.Parallel()

	o, pool := newMockOnline(t)
	calendar := Calendar{
		Path:     "/user/calendars/test",
		Metadata: CalendarMetadata{Name: "Updated"},
	}
	rawMetadata, err := json.Marshal(calendar.Metadata)
	require.NoError(t, err)

	pool.ExpectQuery(getCalendarQuery).
		WithArgs(calendar.Path.String()).
		WillReturnRows(testCalendarRows(t, calendar.Path, CalendarMetadata{Name: "Original"}))
	pool.ExpectBegin()
	pool.ExpectExec(storeCalendarQuery).
		WithArgs(calendar.Path.String(), rawMetadata).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	pool.ExpectCommit()

	created, err := o.PutCalendar(t.Context(), calendar)
	require.NoError(t, err)
	assert.False(t, created)
	require.NoError(t, pool.ExpectationsWereMet())
}

func TestOnlinePutCalendarRollback(t *testing.T) {
	t.Parallel()

	o, pool := newMockOnline(t)
	calendar := Calendar{
		Path:     "/user/calendars/test",
		Metadata: CalendarMetadata{Name: "Test"},
	}

	pool.ExpectQuery(getCalendarQuery).
		WithArgs(calendar.Path.String()).
		WillReturnRows(pgxmock.NewRows([]string{testColumnPath, testColumnMetadata}))
	pool.ExpectBegin()
	pool.ExpectExec(storeCalendarQuery).
		WithArgs(calendar.Path.String(), []byte(`{"name":"Test"}`)).
		WillReturnError(errOnlineTest)
	pool.ExpectRollback()

	_, err := o.PutCalendar(t.Context(), calendar)
	require.ErrorIs(t, err, errOnlineTest)
	require.NoError(t, pool.ExpectationsWereMet())
}

func TestOnlineGetObjectRef(t *testing.T) {
	t.Parallel()

	o, pool := newMockOnline(t)
	path := Path("/user/calendars/test/event.ics")
	modTime := time.Date(2026, time.January, 15, 10, 30, 0, 0, time.UTC)
	ref := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	etag := s3cache.Hash("etag")

	pool.ExpectQuery(getObjectQuery).
		WithArgs(path.String()).
		WillReturnRows(testObjectRefRows(path, modTime, ref, etag))

	got, err := o.getObjectRef(t.Context(), path)
	require.NoError(t, err)
	assert.Equal(t, ObjectRef{Path: path, ModTime: modTime, Ref: ref, ETag: etag}, got)
	require.NoError(t, pool.ExpectationsWereMet())
}

func TestOnlineGetObjectRefNotFound(t *testing.T) {
	t.Parallel()

	o, pool := newMockOnline(t)
	path := Path("/user/calendars/test/missing.ics")

	pool.ExpectQuery(getObjectQuery).
		WithArgs(path.String()).
		WillReturnRows(pgxmock.NewRows([]string{testColumnPath, testColumnModTime, testColumnRef, testColumnETag}))

	_, err := o.getObjectRef(t.Context(), path)
	require.ErrorIs(t, err, ErrCalendarObjectNotFound)
	require.NoError(t, pool.ExpectationsWereMet())
}

func TestOnlineListCalendarObjectsAll(t *testing.T) {
	t.Parallel()

	o, pool := newMockOnline(t)

	pool.ExpectQuery(listObjectQuery).
		WillReturnRows(pgxmock.NewRows([]string{testColumnPath, testColumnModTime, testColumnRef, testColumnETag}))

	objects, err := o.ListCalendarObjects(t.Context(), "")
	require.NoError(t, err)
	assert.Empty(t, objects)
	require.NoError(t, pool.ExpectationsWereMet())
}

func TestOnlineListCalendarObjectsRange(t *testing.T) {
	t.Parallel()

	o, pool := newMockOnline(t)
	base := Path("/user/calendars/test")

	pool.ExpectQuery(rangeObjectQuery).
		WithArgs(base.String(), prefixEnd(base).String()).
		WillReturnRows(pgxmock.NewRows([]string{testColumnPath, testColumnModTime, testColumnRef, testColumnETag}))

	objects, err := o.ListCalendarObjects(t.Context(), base)
	require.NoError(t, err)
	assert.Empty(t, objects)
	require.NoError(t, pool.ExpectationsWereMet())
}

func TestOnlinePutCalendarObject(t *testing.T) {
	t.Parallel()

	o, pool := newMockOnline(t)
	blob := newMemBlob()
	o.objectCache = blob

	object := Object{
		Path:    "/user/calendars/test/event.ics",
		ModTime: time.Date(2026, time.January, 15, 10, 30, 0, 0, time.UTC),
		Data:    []byte("calendar object"),
	}
	etag := s3cache.HashBytes(object.Data)

	pool.ExpectQuery(getObjectQuery).
		WithArgs(object.Path.String()).
		WillReturnRows(pgxmock.NewRows([]string{testColumnPath, testColumnModTime, testColumnRef, testColumnETag}))
	pool.ExpectBegin()
	pool.ExpectExec(storeObjectQuery).
		WithArgs(object.Path.String(), object.ModTime, pgxmock.AnyArg(), etag.String()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	pool.ExpectCommit()

	created, err := o.PutCalendarObject(t.Context(), object)
	require.NoError(t, err)
	assert.True(t, created)
	require.Len(t, blob.data, 1)
	require.NoError(t, pool.ExpectationsWereMet())
}

func TestOnlineDeleteCalendarObject(t *testing.T) {
	t.Parallel()

	o, pool := newMockOnline(t)
	blob := newMemBlob()
	o.objectCache = blob

	path := Path("/user/calendars/test/event.ics")
	modTime := time.Date(2026, time.January, 15, 10, 30, 0, 0, time.UTC)
	ref := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	etag := s3cache.Hash("etag")
	blob.data[ref.String()] = []byte("calendar object")

	pool.ExpectQuery(getObjectQuery).
		WithArgs(path.String()).
		WillReturnRows(testObjectRefRows(path, modTime, ref, etag))
	pool.ExpectBegin()
	pool.ExpectExec(deleteObjectQuery).
		WithArgs(path.String()).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))
	pool.ExpectCommit()

	require.NoError(t, o.DeleteCalendarObject(t.Context(), path))
	assert.Empty(t, blob.data)
	require.NoError(t, pool.ExpectationsWereMet())
}

func TestOnlineStoreObjectRef(t *testing.T) {
	t.Parallel()

	o, pool := newMockOnline(t)
	tx := beginTestTransaction(t, o, pool)

	ref := ObjectRef{
		Path:    "/user/calendars/test/event.ics",
		ModTime: time.Date(2026, time.January, 15, 10, 30, 0, 0, time.UTC),
		Ref:     uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		ETag:    s3cache.Hash("etag"),
	}

	pool.ExpectExec(storeObjectQuery).
		WithArgs(ref.Path.String(), ref.ModTime, pgtype.UUID{Bytes: ref.Ref, Valid: true}, ref.ETag.String()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	pool.ExpectCommit()

	require.NoError(t, tx.StoreObjectRef(t.Context(), ref))
	require.NoError(t, tx.Commit(t.Context()))
	require.NoError(t, pool.ExpectationsWereMet())
}

func TestOnlineDeleteObjectRef(t *testing.T) {
	t.Parallel()

	o, pool := newMockOnline(t)
	tx := beginTestTransaction(t, o, pool)
	path := Path("/user/calendars/test/event.ics")

	pool.ExpectExec(deleteObjectQuery).
		WithArgs(path.String()).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))
	pool.ExpectCommit()

	require.NoError(t, tx.DeleteObjectRef(t.Context(), path))
	require.NoError(t, tx.Commit(t.Context()))
	require.NoError(t, pool.ExpectationsWereMet())
}

func TestOnlineBeginError(t *testing.T) {
	t.Parallel()

	o, pool := newMockOnline(t)

	pool.ExpectBegin().WillReturnError(errOnlineTest)

	_, err := o.Begin(t.Context())
	require.ErrorIs(t, err, errOnlineTest)
	require.ErrorContains(t, err, "could not begin psql transaction")
	require.NoError(t, pool.ExpectationsWereMet())
}

func TestOnlineCommitError(t *testing.T) {
	t.Parallel()

	o, pool := newMockOnline(t)
	tx := beginTestTransaction(t, o, pool)

	pool.ExpectCommit().WillReturnError(errOnlineTest)

	err := tx.Commit(t.Context())
	require.ErrorIs(t, err, errOnlineTest)
	require.ErrorContains(t, err, "error while committing transaction")
	require.NoError(t, pool.ExpectationsWereMet())
}

func TestOnlineRollback(t *testing.T) {
	t.Parallel()

	o, pool := newMockOnline(t)
	tx := beginTestTransaction(t, o, pool)

	pool.ExpectRollback()

	require.NoError(t, tx.Rollback(t.Context()))
	require.NoError(t, pool.ExpectationsWereMet())
}

func TestOnlineRollbackError(t *testing.T) {
	t.Parallel()

	o, pool := newMockOnline(t)
	tx := beginTestTransaction(t, o, pool)

	pool.ExpectRollback().WillReturnError(errOnlineTest)

	err := tx.Rollback(t.Context())
	require.ErrorIs(t, err, errOnlineTest)
	require.ErrorContains(t, err, "error while rolling back transaction")
	require.NoError(t, pool.ExpectationsWereMet())
}

func TestOnlinePing(t *testing.T) {
	t.Parallel()

	o, pool := newMockOnline(t)

	pool.ExpectPing().WillReturnError(errOnlineTest)

	err := o.Ping(t.Context())
	require.ErrorIs(t, err, errOnlineTest)
	require.ErrorContains(t, err, "error while pinging psql")
	require.NoError(t, pool.ExpectationsWereMet())
}
