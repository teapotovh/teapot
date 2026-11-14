package backend

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"testing"
	"time"

	ics "github.com/arran4/golang-ical"
	"github.com/stretchr/testify/require"

	"github.com/teapotovh/teapot/lib/httpauth"
	"github.com/teapotovh/teapot/lib/webdav"
	"github.com/teapotovh/teapot/lib/webdav/caldav"
	daverr "github.com/teapotovh/teapot/lib/webdav/error"
	"github.com/teapotovh/teapot/service/calendar/store"
)

const testObjectPath = "/user/calendars/test/event-1.ics"

func newTestBackend(t *testing.T) *Backend {
	t.Helper()

	logger := slog.New(slog.DiscardHandler)

	return NewBackend(store.NewMem(), logger)
}

func testContext() context.Context {
	return httpauth.NewAuthContext(context.Background(), &httpauth.Auth{Username: "user"})
}

func newTestEvent(t *testing.T, uid, summary string, start, end time.Time) *ics.Calendar {
	t.Helper()

	cal := ics.NewCalendar()
	event := cal.AddEvent(uid)
	event.SetStartAt(start)
	event.SetEndAt(end)

	if summary != "" {
		event.SetSummary(summary)
	}

	return cal
}

func TestCurrentUserPrincipal(t *testing.T) {
	t.Parallel()

	b := newTestBackend(t)

	principal, err := b.CurrentUserPrincipal(testContext())
	require.NoError(t, err)
	require.Equal(t, "/user", principal)

	_, err = b.CurrentUserPrincipal(context.Background())
	require.ErrorIs(t, err, ErrMissingAuthentication)
}

func TestCalendarHomeSetPath(t *testing.T) {
	t.Parallel()

	b := newTestBackend(t)

	path, err := b.CalendarHomeSetPath(testContext())
	require.NoError(t, err)
	require.Equal(t, "/user/calendars/", path)

	_, err = b.CalendarHomeSetPath(context.Background())
	require.ErrorIs(t, err, ErrMissingAuthentication)
}

func TestCalendarLifecycle(t *testing.T) {
	t.Parallel()

	ctx := testContext()
	b := newTestBackend(t)

	calendar := &caldav.Calendar{
		Path:        "/user/calendars/test/",
		Name:        "Test calendar",
		Description: "A calendar used in tests",
		Color:       "#ff0000",
	}

	require.ErrorIs(t, b.CreateCalendar(ctx, nil), ErrUnexpectedNilCalendar)
	require.NoError(t, b.CreateCalendar(ctx, calendar))
	require.Equal(t, SupportedComponentSet, calendar.SupportedComponentSet)

	got, err := b.GetCalendar(ctx, "/user/calendars/test/")
	require.NoError(t, err)
	require.Equal(t, "/user/calendars/test", got.Path)
	require.Equal(t, "Test calendar", got.Name)
	require.Equal(t, "A calendar used in tests", got.Description)
	require.Equal(t, "#ff0000", got.Color)
	require.Equal(t, SupportedComponentSet, got.SupportedComponentSet)

	calendars, err := b.ListCalendars(ctx)
	require.NoError(t, err)
	require.Len(t, calendars, 1)
	require.Equal(t, "Test calendar", calendars[0].Name)
	require.Equal(t, "/user/calendars/test", calendars[0].Path)

	require.ErrorIs(t, b.CreateCalendar(ctx, calendar), store.ErrCalendarAlreadyExists)

	_, err = b.GetCalendar(ctx, "/user/calendars/missing/")
	require.ErrorIs(t, err, store.ErrCalendarNotFound)
}

func TestCalendarObjectRoundTrip(t *testing.T) {
	t.Parallel()

	ctx := testContext()
	b := newTestBackend(t)

	path := testObjectPath
	cal := newTestEvent(
		t,
		"event-1",
		"Event 1",
		time.Date(2026, time.January, 16, 12, 0, 0, 0, time.UTC),
		time.Date(2026, time.January, 16, 13, 0, 0, 0, time.UTC),
	)

	created, wasCreated, err := b.PutCalendarObject(ctx, path, cal, nil)
	require.NoError(t, err)
	require.True(t, wasCreated)
	require.Equal(t, path, created.Path)
	require.NotEmpty(t, created.ETag)
	require.Positive(t, created.ContentLength)
	require.False(t, created.ModTime.IsZero())
	require.Len(t, created.Data.Events(), 1)

	got, err := b.GetCalendarObject(ctx, path, nil)
	require.NoError(t, err)
	require.Equal(t, path, got.Path)
	require.Equal(t, created.ETag, got.ETag)
	require.Equal(t, created.ContentLength, got.ContentLength)
	require.Len(t, got.Data.Events(), 1)
	require.Equal(t, "event-1", got.Data.Events()[0].Id())

	objects, err := b.ListCalendarObjects(ctx, "/user/calendars/test/", nil)
	require.NoError(t, err)
	require.Len(t, objects, 1)
	require.Equal(t, path, objects[0].Path)
	require.Equal(t, created.ETag, objects[0].ETag)

	_, _, err = b.PutCalendarObject(ctx, path, nil, nil)
	require.ErrorIs(t, err, ErrUnexpectedNilObject)

	require.NoError(t, b.DeleteCalendarObject(ctx, path))

	err = b.DeleteCalendarObject(ctx, path)
	require.ErrorIs(t, err, store.ErrCalendarObjectNotFound)
	require.Equal(t, http.StatusNotFound, daverr.HTTPErrorFromError(err).Code)

	_, err = b.GetCalendarObject(ctx, path, nil)
	require.ErrorIs(t, err, store.ErrCalendarObjectNotFound)

	objects, err = b.ListCalendarObjects(ctx, "/user/calendars/test/", nil)
	require.NoError(t, err)
	require.Empty(t, objects)
}

func TestQueryCalendarObjects(t *testing.T) {
	t.Parallel()

	ctx := testContext()
	b := newTestBackend(t)

	inside := "/user/calendars/test/inside.ics"
	outside := "/user/calendars/test/outside.ics"

	_, _, err := b.PutCalendarObject(ctx, inside, newTestEvent(
		t,
		"inside",
		"Inside the range",
		time.Date(2026, time.January, 4, 12, 0, 0, 0, time.UTC),
		time.Date(2026, time.January, 4, 13, 0, 0, 0, time.UTC),
	), nil)
	require.NoError(t, err)

	_, _, err = b.PutCalendarObject(ctx, outside, newTestEvent(
		t,
		"outside",
		"Outside the range",
		time.Date(2026, time.January, 6, 12, 0, 0, 0, time.UTC),
		time.Date(2026, time.January, 6, 13, 0, 0, 0, time.UTC),
	), nil)
	require.NoError(t, err)

	tests := []struct {
		name  string
		query *caldav.CalendarQuery
		want  []string
	}{
		{
			// Mirrors the calendar-query example from RFC 4791 section 7.8.1.
			name: "time range matching a single event",
			query: &caldav.CalendarQuery{
				CompFilter: caldav.CompFilter{
					Name: string(ics.ComponentVCalendar),
					Comps: []caldav.CompFilter{{
						Name:  string(ics.ComponentVEvent),
						Start: time.Date(2026, time.January, 4, 0, 0, 0, 0, time.UTC),
						End:   time.Date(2026, time.January, 5, 0, 0, 0, 0, time.UTC),
					}},
				},
			},
			want: []string{inside},
		},
		{
			name: "time range matching no events",
			query: &caldav.CalendarQuery{
				CompFilter: caldav.CompFilter{
					Name: string(ics.ComponentVCalendar),
					Comps: []caldav.CompFilter{{
						Name:  string(ics.ComponentVEvent),
						Start: time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC),
						End:   time.Date(2026, time.February, 2, 0, 0, 0, 0, time.UTC),
					}},
				},
			},
			want: []string{},
		},
		{
			name: "time range matching all events",
			query: &caldav.CalendarQuery{
				CompFilter: caldav.CompFilter{
					Name: string(ics.ComponentVCalendar),
					Comps: []caldav.CompFilter{{
						Name:  string(ics.ComponentVEvent),
						Start: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
						End:   time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC),
					}},
				},
			},
			want: []string{inside, outside},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			objects, err := b.QueryCalendarObjects(ctx, "/user/calendars/test/", test.query)
			require.NoError(t, err)

			paths := make([]string, 0, len(objects))
			for _, object := range objects {
				paths = append(paths, object.Path)
			}

			require.ElementsMatch(t, test.want, paths)
		})
	}
}

func TestPutCalendarObjectIfMatchWrongETag(t *testing.T) {
	t.Parallel()

	ctx := testContext()
	b := newTestBackend(t)

	path := testObjectPath
	cal := newTestEvent(
		t,
		"event-1",
		"Event 1",
		time.Date(2026, time.January, 16, 12, 0, 0, 0, time.UTC),
		time.Date(2026, time.January, 16, 13, 0, 0, 0, time.UTC),
	)

	_, _, err := b.PutCalendarObject(ctx, path, cal, nil)
	require.NoError(t, err)

	opts := &caldav.PutCalendarObjectOptions{
		IfMatch: webdav.ConditionalMatch(`"not-the-current-etag"`),
	}

	_, _, err = b.PutCalendarObject(ctx, path, cal, opts)
	require.ErrorIs(t, err, ErrETagDidNotMatch)

	httpErr := &daverr.HTTPError{}
	require.ErrorAs(t, err, &httpErr)
	require.Equal(t, http.StatusPreconditionFailed, httpErr.Code)
}

func TestPutCalendarObjectIfMatchMalformedETag(t *testing.T) {
	t.Parallel()

	ctx := testContext()
	b := newTestBackend(t)

	path := testObjectPath
	cal := newTestEvent(
		t,
		"event-1",
		"Event 1",
		time.Date(2026, time.January, 16, 12, 0, 0, 0, time.UTC),
		time.Date(2026, time.January, 16, 13, 0, 0, 0, time.UTC),
	)

	_, _, err := b.PutCalendarObject(ctx, path, cal, nil)
	require.NoError(t, err)

	opts := &caldav.PutCalendarObjectOptions{
		IfMatch: webdav.ConditionalMatch("not-a-quoted-etag"),
	}

	_, _, err = b.PutCalendarObject(ctx, path, cal, opts)

	httpErr := &daverr.HTTPError{}
	require.ErrorAs(t, err, &httpErr)
	require.Equal(t, http.StatusPreconditionFailed, httpErr.Code)
}

func TestPutCalendarObjectIfNoneMatchWildcardExisting(t *testing.T) {
	t.Parallel()

	ctx := testContext()
	b := newTestBackend(t)

	path := testObjectPath
	cal := newTestEvent(
		t,
		"event-1",
		"Event 1",
		time.Date(2026, time.January, 16, 12, 0, 0, 0, time.UTC),
		time.Date(2026, time.January, 16, 13, 0, 0, 0, time.UTC),
	)

	_, _, err := b.PutCalendarObject(ctx, path, cal, nil)
	require.NoError(t, err)

	opts := &caldav.PutCalendarObjectOptions{
		IfNoneMatch: webdav.ConditionalMatch("*"),
	}

	_, _, err = b.PutCalendarObject(ctx, path, cal, opts)
	require.ErrorIs(t, err, ErrETagDidNotMatch)

	httpErr := &daverr.HTTPError{}
	require.ErrorAs(t, err, &httpErr)
	require.Equal(t, http.StatusPreconditionFailed, httpErr.Code)
}

func TestPutCalendarObjectIfNoneMatchWildcardNew(t *testing.T) {
	t.Parallel()

	ctx := testContext()
	b := newTestBackend(t)

	path := testObjectPath
	cal := newTestEvent(
		t,
		"event-1",
		"Event 1",
		time.Date(2026, time.January, 16, 12, 0, 0, 0, time.UTC),
		time.Date(2026, time.January, 16, 13, 0, 0, 0, time.UTC),
	)

	opts := &caldav.PutCalendarObjectOptions{
		IfNoneMatch: webdav.ConditionalMatch("*"),
	}

	created, wasCreated, err := b.PutCalendarObject(ctx, path, cal, opts)
	require.NoError(t, err)
	require.True(t, wasCreated)
	require.Equal(t, path, created.Path)
	require.NotEmpty(t, created.ETag)

	stored, err := b.GetCalendarObject(ctx, path, nil)
	require.NoError(t, err)
	require.Equal(t, created.ETag, stored.ETag)
}

func TestPutCalendarObjectUpdateWithMatchingETag(t *testing.T) {
	t.Parallel()

	ctx := testContext()
	b := newTestBackend(t)

	path := testObjectPath
	start := time.Date(2026, time.January, 16, 12, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.January, 16, 13, 0, 0, 0, time.UTC)

	created, wasCreated, err := b.PutCalendarObject(
		ctx,
		path,
		newTestEvent(t, "event-1", "Event 1", start, end),
		nil,
	)
	require.NoError(t, err)
	require.True(t, wasCreated)

	opts := &caldav.PutCalendarObjectOptions{
		IfMatch: webdav.ConditionalMatch(fmt.Sprintf("%q", created.ETag)),
	}

	updated, wasCreated, err := b.PutCalendarObject(
		ctx,
		path,
		newTestEvent(t, "event-1", "Event 1 updated", start, end),
		opts,
	)
	require.NoError(t, err)
	require.False(t, wasCreated)
	require.NotEqual(t, created.ETag, updated.ETag)
	require.Equal(t, "Event 1 updated", updated.Data.Events()[0].GetProperty(ics.ComponentPropertySummary).Value)

	stored, err := b.GetCalendarObject(ctx, path, nil)
	require.NoError(t, err)
	require.Equal(t, updated.ETag, stored.ETag)
	require.Equal(t, "Event 1 updated", stored.Data.Events()[0].GetProperty(ics.ComponentPropertySummary).Value)
}

func TestPutCalendarObjectIfMatchMissing(t *testing.T) {
	t.Parallel()

	ctx := testContext()
	b := newTestBackend(t)

	path := testObjectPath
	cal := newTestEvent(
		t,
		"event-1",
		"Event 1",
		time.Date(2026, time.January, 16, 12, 0, 0, 0, time.UTC),
		time.Date(2026, time.January, 16, 13, 0, 0, 0, time.UTC),
	)

	opts := &caldav.PutCalendarObjectOptions{
		IfMatch: webdav.ConditionalMatch(`"missing"`),
	}

	_, _, err := b.PutCalendarObject(ctx, path, cal, opts)
	requireHTTPStatus(t, err, http.StatusPreconditionFailed)

	_, err = b.GetCalendarObject(ctx, path, nil)
	require.ErrorIs(t, err, store.ErrCalendarObjectNotFound)
}

func TestPutCalendarObjectUpdateWithDifferentUID(t *testing.T) {
	t.Parallel()

	ctx := testContext()
	b := newTestBackend(t)

	path := testObjectPath
	start := time.Date(2026, time.January, 16, 12, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.January, 16, 13, 0, 0, 0, time.UTC)

	_, _, err := b.PutCalendarObject(ctx, path, newTestEvent(t, "event-1", "Event 1", start, end), nil)
	require.NoError(t, err)

	_, _, err = b.PutCalendarObject(ctx, path, newTestEvent(t, "event-2", "Event 2", start, end), nil)
	requireHTTPStatus(t, err, http.StatusConflict)

	stored, err := b.GetCalendarObject(ctx, path, nil)
	require.NoError(t, err)
	require.Equal(t, "event-1", stored.Data.Events()[0].Id())
}

func requireHTTPStatus(t *testing.T, err error, want int) {
	t.Helper()

	httpErr := &daverr.HTTPError{}
	require.ErrorAs(t, err, &httpErr)
	require.Equal(t, want, httpErr.Code)
}
