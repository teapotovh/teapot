package store

import (
	"context"
	"testing"
	"time"

	ics "github.com/arran4/golang-ical"
	"github.com/stretchr/testify/require"

	"github.com/teapotovh/teapot/lib/webdav/caldav"
)

var testModTime = time.Date(2026, time.January, 15, 10, 30, 0, 0, time.UTC)

func newTestCalendar(t *testing.T, summary string) *ics.Calendar {
	t.Helper()

	cal := ics.NewCalendar()
	event := cal.AddEvent("event-1")
	event.SetStartAt(time.Date(2026, time.January, 16, 12, 0, 0, 0, time.UTC))
	event.SetEndAt(time.Date(2026, time.January, 16, 13, 0, 0, 0, time.UTC))

	if summary != "" {
		event.SetSummary(summary)
	}

	return cal
}

func newTestObject(t *testing.T, path, summary string) Object {
	t.Helper()

	obj, err := SerializeObject(caldav.CalendarObject{
		Path:    path,
		ModTime: testModTime,
		Data:    newTestCalendar(t, summary),
	})
	require.NoError(t, err)

	return *obj
}

func TestMemCalendarLifecycle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	st := NewMem()

	calendar := Calendar{
		Path: "/user/calendars/test",
		Metadata: CalendarMetadata{
			Name:        "Test calendar",
			Description: "A calendar used in tests",
			Color:       "#ff0000",
		},
	}

	require.NoError(t, st.CreateCalendar(ctx, calendar))
	require.ErrorIs(t, st.CreateCalendar(ctx, calendar), ErrCalendarAlreadyExists)

	got, err := st.GetCalendar(ctx, calendar.Path)
	require.NoError(t, err)
	require.Equal(t, calendar, *got)

	_, err = st.GetCalendar(ctx, "/user/calendars/missing")
	require.ErrorIs(t, err, ErrCalendarNotFound)

	calendars, err := st.ListCalendars(ctx, "/user/calendars")
	require.NoError(t, err)
	require.Len(t, calendars, 1)
	require.Equal(t, calendar, calendars[0])

	calendars, err = st.ListCalendars(ctx, "/other")
	require.NoError(t, err)
	require.Empty(t, calendars)
}

func TestMemObjectLifecycle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	st := NewMem()

	object := newTestObject(t, "/user/calendars/test/event-1.ics", "Event 1")

	require.NoError(t, st.CreateCalendarObject(ctx, object))
	require.ErrorIs(t, st.CreateCalendarObject(ctx, object), ErrCalendarObjectAlreadyExists)

	got, err := st.GetCalendarObject(ctx, object.Path)
	require.NoError(t, err)
	require.Equal(t, object.Path, got.Path)
	require.Equal(t, object.ModTime, got.ModTime)
	require.Equal(t, object.Data, got.Data)
	require.Equal(t, object.ETag, got.ETag)
	require.NotEmpty(t, got.ETag)

	other := newTestObject(t, "/user/calendars/other/event-2.ics", "Event 2")
	require.NoError(t, st.CreateCalendarObject(ctx, other))

	objects, err := st.ListCalendarObjects(ctx, "/user/calendars/test")
	require.NoError(t, err)
	require.Len(t, objects, 1)
	require.Equal(t, object, objects[0])

	require.NoError(t, st.DeleteCalendarObject(ctx, object.Path))

	_, err = st.GetCalendarObject(ctx, object.Path)
	require.ErrorIs(t, err, ErrCalendarObjectNotFound)

	require.ErrorIs(t, st.DeleteCalendarObject(ctx, object.Path), ErrCalendarObjectNotFound)
}

func TestSerializeObjectDeterministicETag(t *testing.T) {
	t.Parallel()

	object := caldav.CalendarObject{
		Path:    "/user/calendars/test/event-1.ics",
		ModTime: testModTime,
		Data:    newTestCalendar(t, "Event 1"),
	}

	first, err := SerializeObject(object)
	require.NoError(t, err)

	second, err := SerializeObject(object)
	require.NoError(t, err)

	require.NotEmpty(t, first.ETag)
	require.Equal(t, first.ETag, second.ETag)
	require.Equal(t, first.Data, second.Data)
	require.Equal(t, Path(object.Path), first.Path)
	require.Equal(t, object.ModTime, first.ModTime)
}

func TestSerializeObjectETagChangesWithData(t *testing.T) {
	t.Parallel()

	base := caldav.CalendarObject{
		Path:    "/user/calendars/test/event-1.ics",
		ModTime: testModTime,
		Data:    newTestCalendar(t, "Event 1"),
	}
	updated := base
	updated.Data = newTestCalendar(t, "Event 1 updated")

	first, err := SerializeObject(base)
	require.NoError(t, err)

	second, err := SerializeObject(updated)
	require.NoError(t, err)

	require.NotEqual(t, first.ETag, second.ETag)
	require.NotEqual(t, first.Data, second.Data)
}

func TestObjectCalendarAndSize(t *testing.T) {
	t.Parallel()

	object := newTestObject(t, "/user/calendars/test/event-1.ics", "Event 1")

	require.Equal(t, int64(len(object.Data)), object.Size())
	require.NotEmpty(t, object.Data)

	cal, err := object.Calendar()
	require.NoError(t, err)
	require.Len(t, cal.Events(), 1)

	event := cal.Events()[0]
	require.Equal(t, "event-1", event.Id())

	start, err := event.GetStartAt()
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, time.January, 16, 12, 0, 0, 0, time.UTC), start.UTC())

	end, err := event.GetEndAt()
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, time.January, 16, 13, 0, 0, 0, time.UTC), end.UTC())
}

func TestObjectCalendarInvalidData(t *testing.T) {
	t.Parallel()

	object := Object{
		Path: "/user/calendars/test/invalid.ics",
		Data: []byte("not an icalendar object"),
	}

	_, err := object.Calendar()
	require.Error(t, err)
}

func TestPathPrefixFromString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		want Path
	}{
		{name: "absolute path", path: "/user/calendars/test", want: Path("/user/calendars/test")},
		{name: "relative path", path: "user/calendars", want: Path("user/calendars")},
		{name: "empty path", path: "", want: Path("")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := PrefixFromString(test.path)
			require.NoError(t, err)
			require.NotNil(t, got)
			require.Equal(t, test.want, *got)
		})
	}
}

func TestPathLess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		left  Path
		right Path
		want  bool
	}{
		{name: "lexicographically before", left: "/user/calendars/a", right: "/user/calendars/b", want: true},
		{name: "lexicographically after", left: "/user/calendars/b", right: "/user/calendars/a", want: false},
		{name: "equal", left: "/user/calendars/a", right: "/user/calendars/a", want: false},
		{name: "shorter prefix", left: "/user", right: "/user/calendars", want: true},
		{name: "empty before non empty", left: "", right: "/user", want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, test.want, test.left.Less(test.right))
		})
	}
}

func TestPathString(t *testing.T) {
	t.Parallel()

	require.Equal(t, "/user/calendars/test", Path("/user/calendars/test").String())
}
