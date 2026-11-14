package backend

import (
	"context"
	"encoding/xml"
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

	_, err := b.PutCalendar(ctx, nil)
	require.ErrorIs(t, err, ErrUnexpectedNilCalendar)

	created, err := b.PutCalendar(ctx, calendar)
	require.NoError(t, err)
	require.True(t, created)
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

	created, err = b.PutCalendar(ctx, calendar)
	require.NoError(t, err)
	require.False(t, created)

	_, err = b.GetCalendar(ctx, "/user/calendars/missing/")
	require.ErrorIs(t, err, store.ErrCalendarNotFound)
}

func TestGetCalendarMissing(t *testing.T) {
	t.Parallel()

	b := newTestBackend(t)

	_, err := b.GetCalendar(testContext(), "/user/calendars/missing/")
	require.ErrorIs(t, err, store.ErrCalendarNotFound)
	requireHTTPStatus(t, err, http.StatusNotFound)
}

func TestPutCalendar(t *testing.T) {
	t.Parallel()

	ctx := testContext()
	b := newTestBackend(t)

	calendarPath := "/user/calendars/test/"
	details := xml.Name{Space: "DAV:", Local: "details"}
	details2 := xml.Name{Space: "DAV:", Local: "details2"}

	calendar := &caldav.Calendar{
		Path: calendarPath,
		Name: "Test calendar",
		DeadProperties: map[xml.Name]string{
			details:  `<details xmlns="DAV:">My Name</details>`,
			details2: `<details2 xmlns="DAV:">My Name 2</details2>`,
		},
	}
	created, err := b.PutCalendar(ctx, calendar)
	require.NoError(t, err)
	require.True(t, created)

	got, err := b.GetCalendar(ctx, calendarPath)
	require.NoError(t, err)
	require.Equal(t, "Test calendar", got.Name)
	require.Equal(t, calendar.DeadProperties, got.DeadProperties)

	replacement := &caldav.Calendar{
		Path: calendarPath,
		Name: "Renamed calendar",
		DeadProperties: map[xml.Name]string{
			details2: `<details2 xmlns="DAV:">My Name 3</details2>`,
		},
	}
	created, err = b.PutCalendar(ctx, replacement)
	require.NoError(t, err)
	require.False(t, created)

	got, err = b.GetCalendar(ctx, calendarPath)
	require.NoError(t, err)
	require.Equal(t, "Renamed calendar", got.Name)
	require.Equal(t, replacement.DeadProperties, got.DeadProperties)
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

	created, wasCreated, err := b.PutCalendarObject(ctx, path, cal, webdav.Preconditions{})
	require.NoError(t, err)
	require.True(t, wasCreated)
	require.Equal(t, path, created.Path)
	require.NotEmpty(t, created.ETag)
	require.Positive(t, created.ContentLength)
	require.False(t, created.ModTime.IsZero())
	require.Len(t, created.Data.Events(), 1)

	got, _, err := b.GetCalendarObject(ctx, path, nil, webdav.Preconditions{})
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

	_, _, err = b.PutCalendarObject(ctx, path, nil, webdav.Preconditions{})
	require.ErrorIs(t, err, ErrUnexpectedNilObject)

	require.NoError(t, b.DeleteCalendarObject(ctx, path, webdav.Preconditions{}))

	err = b.DeleteCalendarObject(ctx, path, webdav.Preconditions{})
	require.ErrorIs(t, err, store.ErrCalendarObjectNotFound)
	require.Equal(t, http.StatusNotFound, daverr.HTTPErrorFromError(err).Code)

	_, _, err = b.GetCalendarObject(ctx, path, nil, webdav.Preconditions{})
	require.ErrorIs(t, err, store.ErrCalendarObjectNotFound)

	objects, err = b.ListCalendarObjects(ctx, "/user/calendars/test/", nil)
	require.NoError(t, err)
	require.Empty(t, objects)
}

func TestPutCalendarObjectSupportedComponentSet(t *testing.T) {
	t.Parallel()

	ctx := testContext()
	b := newTestBackend(t)

	_, err := b.PutCalendar(ctx, &caldav.Calendar{
		Path:                  "/user/calendars/test/",
		SupportedComponentSet: []string{string(ics.ComponentVEvent)},
	})
	require.NoError(t, err)

	todo := ics.NewCalendar()
	todo.AddTodo("todo-1").SetSummary("Todo 1")

	_, _, err = b.PutCalendarObject(ctx, testObjectPath, todo, webdav.Preconditions{})
	requireHTTPStatus(t, err, http.StatusForbidden)

	event := newTestEvent(
		t,
		"event-1",
		"Event 1",
		time.Date(2026, time.January, 16, 12, 0, 0, 0, time.UTC),
		time.Date(2026, time.January, 16, 13, 0, 0, 0, time.UTC),
	)

	_, _, err = b.PutCalendarObject(ctx, testObjectPath, event, webdav.Preconditions{})
	require.NoError(t, err)

	withTimezone := newTestEvent(
		t,
		"event-1",
		"Event 1",
		time.Date(2026, time.January, 16, 12, 0, 0, 0, time.UTC),
		time.Date(2026, time.January, 16, 13, 0, 0, 0, time.UTC),
	)
	withTimezone.AddTimezone("Europe/Rome")

	_, _, err = b.PutCalendarObject(ctx, testObjectPath, withTimezone, webdav.Preconditions{})
	require.NoError(t, err)
}

func TestDeleteCalendarObjectPreconditions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		pre         func(etag string) webdav.Preconditions
		wantDeleted bool
	}{
		{
			name: "if-match matches",
			pre: func(etag string) webdav.Preconditions {
				return webdav.Preconditions{
					IfMatch: webdav.ConditionalMatch(fmt.Sprintf("%q", etag)),
				}
			},
			wantDeleted: true,
		},
		{
			name: "if-match does not match",
			pre: func(string) webdav.Preconditions {
				return webdav.Preconditions{
					IfMatch: webdav.ConditionalMatch(`"missing"`),
				}
			},
		},
		{
			name: "if-match wildcard",
			pre: func(string) webdav.Preconditions {
				return webdav.Preconditions{
					IfMatch: webdav.ConditionalMatch("*"),
				}
			},
			wantDeleted: true,
		},
		{
			name: "if-none-match matches",
			pre: func(etag string) webdav.Preconditions {
				return webdav.Preconditions{
					IfNoneMatch: webdav.ConditionalMatch(fmt.Sprintf("%q", etag)),
				}
			},
		},
		{
			name: "if-none-match does not match",
			pre: func(string) webdav.Preconditions {
				return webdav.Preconditions{
					IfNoneMatch: webdav.ConditionalMatch(`"missing"`),
				}
			},
			wantDeleted: true,
		},
		{
			name: "if-none-match wildcard",
			pre: func(string) webdav.Preconditions {
				return webdav.Preconditions{
					IfNoneMatch: webdav.ConditionalMatch("*"),
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend(t)
			created := putTestObject(t, b)

			err := b.DeleteCalendarObject(testContext(), testObjectPath, tt.pre(created.ETag))
			if tt.wantDeleted {
				require.NoError(t, err)

				_, _, err = b.GetCalendarObject(testContext(), testObjectPath, nil, webdav.Preconditions{})
				require.ErrorIs(t, err, store.ErrCalendarObjectNotFound)

				return
			}

			require.ErrorIs(t, err, webdav.ErrPreconditionFailed)

			stored, _, err := b.GetCalendarObject(testContext(), testObjectPath, nil, webdav.Preconditions{})
			require.NoError(t, err)
			require.Equal(t, created.ETag, stored.ETag)
		})
	}
}

func TestDeleteCalendarObjectIfMatchMissing(t *testing.T) {
	t.Parallel()

	b := newTestBackend(t)

	pre := webdav.Preconditions{
		IfMatch: webdav.ConditionalMatch("*"),
	}

	err := b.DeleteCalendarObject(testContext(), testObjectPath, pre)
	require.ErrorIs(t, err, webdav.ErrPreconditionFailed)
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
	), webdav.Preconditions{})
	require.NoError(t, err)

	_, _, err = b.PutCalendarObject(ctx, outside, newTestEvent(
		t,
		"outside",
		"Outside the range",
		time.Date(2026, time.January, 6, 12, 0, 0, 0, time.UTC),
		time.Date(2026, time.January, 6, 13, 0, 0, 0, time.UTC),
	), webdav.Preconditions{})
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

	_, _, err := b.PutCalendarObject(ctx, path, cal, webdav.Preconditions{})
	require.NoError(t, err)

	pre := webdav.Preconditions{
		IfMatch: webdav.ConditionalMatch(`"not-the-current-etag"`),
	}

	_, _, err = b.PutCalendarObject(ctx, path, cal, pre)
	require.ErrorIs(t, err, webdav.ErrPreconditionFailed)
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

	_, _, err := b.PutCalendarObject(ctx, path, cal, webdav.Preconditions{})
	require.NoError(t, err)

	pre := webdav.Preconditions{
		IfNoneMatch: webdav.ConditionalMatch("*"),
	}

	_, _, err = b.PutCalendarObject(ctx, path, cal, pre)
	require.ErrorIs(t, err, webdav.ErrPreconditionFailed)
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

	pre := webdav.Preconditions{
		IfNoneMatch: webdav.ConditionalMatch("*"),
	}

	created, wasCreated, err := b.PutCalendarObject(ctx, path, cal, pre)
	require.NoError(t, err)
	require.True(t, wasCreated)
	require.Equal(t, path, created.Path)
	require.NotEmpty(t, created.ETag)

	stored, _, err := b.GetCalendarObject(ctx, path, nil, webdav.Preconditions{})
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
		webdav.Preconditions{},
	)
	require.NoError(t, err)
	require.True(t, wasCreated)

	pre := webdav.Preconditions{
		IfMatch: webdav.ConditionalMatch(fmt.Sprintf("%q", created.ETag)),
	}

	updated, wasCreated, err := b.PutCalendarObject(
		ctx,
		path,
		newTestEvent(t, "event-1", "Event 1 updated", start, end),
		pre,
	)
	require.NoError(t, err)
	require.False(t, wasCreated)
	require.NotEqual(t, created.ETag, updated.ETag)
	require.Equal(t, "Event 1 updated", updated.Data.Events()[0].GetProperty(ics.ComponentPropertySummary).Value)

	stored, _, err := b.GetCalendarObject(ctx, path, nil, webdav.Preconditions{})
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

	pre := webdav.Preconditions{
		IfMatch: webdav.ConditionalMatch(`"missing"`),
	}

	_, _, err := b.PutCalendarObject(ctx, path, cal, pre)
	require.ErrorIs(t, err, webdav.ErrPreconditionFailed)

	_, _, err = b.GetCalendarObject(ctx, path, nil, webdav.Preconditions{})
	require.ErrorIs(t, err, store.ErrCalendarObjectNotFound)
}

func testEvent(t *testing.T, summary string) *ics.Calendar {
	t.Helper()

	return newTestEvent(
		t,
		"event-1",
		summary,
		time.Date(2026, time.January, 16, 12, 0, 0, 0, time.UTC),
		time.Date(2026, time.January, 16, 13, 0, 0, 0, time.UTC),
	)
}

func putTestObject(t *testing.T, b *Backend) *caldav.CalendarObject {
	t.Helper()

	created, wasCreated, err := b.PutCalendarObject(
		testContext(),
		testObjectPath,
		testEvent(t, "Event 1"),
		webdav.Preconditions{},
	)
	require.NoError(t, err)
	require.True(t, wasCreated)

	return created
}

func TestGetCalendarObjectPreconditions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pre     func(etag string) webdav.Preconditions
		outcome webdav.Outcome
	}{
		{
			name: "if-match matches",
			pre: func(etag string) webdav.Preconditions {
				return webdav.Preconditions{IfMatch: webdav.ConditionalMatch(fmt.Sprintf("%q", etag))}
			},
			outcome: webdav.Proceed,
		},
		{
			name: "if-match does not match",
			pre: func(string) webdav.Preconditions {
				return webdav.Preconditions{IfMatch: webdav.ConditionalMatch(`"missing"`)}
			},
			outcome: webdav.PreconditionFailed,
		},
		{
			name: "if-none-match matches",
			pre: func(etag string) webdav.Preconditions {
				return webdav.Preconditions{IfNoneMatch: webdav.ConditionalMatch(fmt.Sprintf("%q", etag))}
			},
			outcome: webdav.NotModified,
		},
		{
			name: "if-none-match does not match",
			pre: func(string) webdav.Preconditions {
				return webdav.Preconditions{IfNoneMatch: webdav.ConditionalMatch(`"missing"`)}
			},
			outcome: webdav.Proceed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend(t)
			created := putTestObject(t, b)

			got, outcome, err := b.GetCalendarObject(
				testContext(),
				testObjectPath,
				nil,
				tt.pre(created.ETag),
			)
			require.NoError(t, err)
			require.Equal(t, tt.outcome, outcome)
			require.Equal(t, created.ETag, got.ETag)
		})
	}
}

func TestPutCalendarObjectIfMatchETagList(t *testing.T) {
	t.Parallel()

	b := newTestBackend(t)
	created := putTestObject(t, b)

	pre := webdav.Preconditions{
		IfMatch: webdav.ConditionalMatch(fmt.Sprintf(`"missing", %q`, created.ETag)),
	}

	updated, wasCreated, err := b.PutCalendarObject(
		testContext(),
		testObjectPath,
		testEvent(t, "Event 1 updated"),
		pre,
	)
	require.NoError(t, err)
	require.False(t, wasCreated)
	require.NotEqual(t, created.ETag, updated.ETag)
}

func TestPutCalendarObjectIfMatchWeakETag(t *testing.T) {
	t.Parallel()

	b := newTestBackend(t)
	created := putTestObject(t, b)

	pre := webdav.Preconditions{
		IfMatch: webdav.ConditionalMatch(fmt.Sprintf("W/%q", created.ETag)),
	}

	_, _, err := b.PutCalendarObject(
		testContext(),
		testObjectPath,
		testEvent(t, "Event 1 updated"),
		pre,
	)
	require.ErrorIs(t, err, webdav.ErrPreconditionFailed)
}

func TestPutCalendarObjectIfNoneMatchETagList(t *testing.T) {
	t.Parallel()

	b := newTestBackend(t)
	created := putTestObject(t, b)

	pre := webdav.Preconditions{
		IfNoneMatch: webdav.ConditionalMatch(fmt.Sprintf(`"missing", %q`, created.ETag)),
	}

	_, _, err := b.PutCalendarObject(
		testContext(),
		testObjectPath,
		testEvent(t, "Event 1 updated"),
		pre,
	)
	require.ErrorIs(t, err, webdav.ErrPreconditionFailed)

	stored, _, err := b.GetCalendarObject(testContext(), testObjectPath, nil, webdav.Preconditions{})
	require.NoError(t, err)
	require.Equal(t, created.ETag, stored.ETag)
	require.Equal(t, "Event 1", stored.Data.Events()[0].GetProperty(ics.ComponentPropertySummary).Value)
}

func TestPutCalendarObjectIfNoneMatchETagListNoMatch(t *testing.T) {
	t.Parallel()

	b := newTestBackend(t)
	putTestObject(t, b)

	pre := webdav.Preconditions{
		IfNoneMatch: webdav.ConditionalMatch(`"missing", "other"`),
	}

	_, _, err := b.PutCalendarObject(
		testContext(),
		testObjectPath,
		testEvent(t, "Event 1 updated"),
		pre,
	)
	require.NoError(t, err)
}

func TestPutCalendarObjectIfNoneMatchWeakETag(t *testing.T) {
	t.Parallel()

	b := newTestBackend(t)
	created := putTestObject(t, b)

	pre := webdav.Preconditions{
		IfNoneMatch: webdav.ConditionalMatch(fmt.Sprintf("W/%q", created.ETag)),
	}

	_, _, err := b.PutCalendarObject(
		testContext(),
		testObjectPath,
		testEvent(t, "Event 1 updated"),
		pre,
	)
	require.ErrorIs(t, err, webdav.ErrPreconditionFailed)

	stored, _, err := b.GetCalendarObject(testContext(), testObjectPath, nil, webdav.Preconditions{})
	require.NoError(t, err)
	require.Equal(t, created.ETag, stored.ETag)
	require.Equal(t, "Event 1", stored.Data.Events()[0].GetProperty(ics.ComponentPropertySummary).Value)
}

func TestPutCalendarObjectIfMatchETagListNoMatch(t *testing.T) {
	t.Parallel()

	b := newTestBackend(t)
	putTestObject(t, b)

	pre := webdav.Preconditions{
		IfMatch: webdav.ConditionalMatch(`"missing", "other"`),
	}

	_, _, err := b.PutCalendarObject(
		testContext(),
		testObjectPath,
		testEvent(t, "Event 1 updated"),
		pre,
	)
	require.ErrorIs(t, err, webdav.ErrPreconditionFailed)
}

func TestPutCalendarObjectIfMatchWeakThenStrongETag(t *testing.T) {
	t.Parallel()

	b := newTestBackend(t)
	created := putTestObject(t, b)

	pre := webdav.Preconditions{
		IfMatch: webdav.ConditionalMatch(fmt.Sprintf(`W/"missing", %q`, created.ETag)),
	}

	_, _, err := b.PutCalendarObject(
		testContext(),
		testObjectPath,
		testEvent(t, "Event 1 updated"),
		pre,
	)
	require.NoError(t, err)
}

func TestPutCalendarObjectIfMatchWildcard(t *testing.T) {
	t.Parallel()

	t.Run("existing object", func(t *testing.T) {
		t.Parallel()

		b := newTestBackend(t)
		putTestObject(t, b)

		pre := webdav.Preconditions{
			IfMatch: webdav.ConditionalMatch("*"),
		}

		_, _, err := b.PutCalendarObject(
			testContext(),
			testObjectPath,
			testEvent(t, "Event 1 updated"),
			pre,
		)
		require.NoError(t, err)
	})

	t.Run("missing object", func(t *testing.T) {
		t.Parallel()

		b := newTestBackend(t)

		pre := webdav.Preconditions{
			IfMatch: webdav.ConditionalMatch("*"),
		}

		_, _, err := b.PutCalendarObject(
			testContext(),
			testObjectPath,
			testEvent(t, "Event 1"),
			pre,
		)
		require.ErrorIs(t, err, webdav.ErrPreconditionFailed)

		_, _, err = b.GetCalendarObject(testContext(), testObjectPath, nil, webdav.Preconditions{})
		require.ErrorIs(t, err, store.ErrCalendarObjectNotFound)
	})
}

func TestPutCalendarObjectIfMatchAndIfNoneMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		sameETag  bool
		wantError bool
	}{
		{name: "both match", sameETag: true, wantError: true},
		{name: "only if-match matches", sameETag: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend(t)
			created := putTestObject(t, b)

			ifNoneMatch := `"missing"`
			if tt.sameETag {
				ifNoneMatch = fmt.Sprintf("%q", created.ETag)
			}

			pre := webdav.Preconditions{
				IfMatch:     webdav.ConditionalMatch(fmt.Sprintf("%q", created.ETag)),
				IfNoneMatch: webdav.ConditionalMatch(ifNoneMatch),
			}

			_, _, err := b.PutCalendarObject(
				testContext(),
				testObjectPath,
				newTestEvent(
					t,
					"event-1",
					"Event 1 updated",
					time.Date(2026, time.January, 16, 12, 0, 0, 0, time.UTC),
					time.Date(2026, time.January, 16, 13, 0, 0, 0, time.UTC),
				),
				pre,
			)

			if tt.wantError {
				require.ErrorIs(t, err, webdav.ErrPreconditionFailed)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestPutCalendarObjectUpdateWithDifferentUID(t *testing.T) {
	t.Parallel()

	ctx := testContext()
	b := newTestBackend(t)

	path := testObjectPath
	start := time.Date(2026, time.January, 16, 12, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.January, 16, 13, 0, 0, 0, time.UTC)

	_, _, err := b.PutCalendarObject(
		ctx,
		path,
		newTestEvent(t, "event-1", "Event 1", start, end),
		webdav.Preconditions{},
	)
	require.NoError(t, err)

	_, _, err = b.PutCalendarObject(
		ctx,
		path,
		newTestEvent(t, "event-2", "Event 2", start, end),
		webdav.Preconditions{},
	)
	requireHTTPStatus(t, err, http.StatusConflict)

	stored, _, err := b.GetCalendarObject(ctx, path, nil, webdav.Preconditions{})
	require.NoError(t, err)
	require.Equal(t, "event-1", stored.Data.Events()[0].Id())
}

func requireHTTPStatus(t *testing.T, err error, want int) {
	t.Helper()

	httpErr := &daverr.HTTPError{}
	require.ErrorAs(t, err, &httpErr)
	require.Equal(t, want, httpErr.Code)
}
