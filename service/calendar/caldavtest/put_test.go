package caldavtest

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func putAndReadBack(t *testing.T, srv *httptest.Server, object string, fixture string, wantFixture string) {
	t.Helper()

	res := putAppleFixture(t, srv, userOne, object, fixture)
	requireSuccess(t, res)

	// The expected readback comes from the Apple Resource fixture in wantFixture.
	res = doRequest(t, srv, http.MethodGet, object)
	requireSuccess(t, res)
	assertCalendarDataEqual(t, res.Body, loadAppleFixture(t, wantFixture, nil))
}

func TestApplePut(t *testing.T) {
	t.Parallel()

	t.Run("vevent", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			object     string
			fixture    string
			wantData   string
			skipReason string
		}{
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/put.xml#L31-L62
				name:     "non-recurring timed event",
				object:   calendarPath(userOne) + object1,
				fixture:  fixtureRecurrenceput1,
				wantData: fixtureRecurrenceput1,
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/put.xml#L63-L90
				name:       "outside calendar collection",
				object:     calendarHome(userOne) + object1,
				fixture:    fixtureRecurrenceput1,
				wantData:   fixtureRecurrenceput1,
				skipReason: "Optional: regular-collection is not planned.",
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/put.xml#L91-L115
				name:     "timed event with timezone",
				object:   calendarPath(userOne) + object2,
				fixture:  "Resource/CalDAV/recurrenceput/20.ics",
				wantData: "Resource/CalDAV/recurrenceput/21.ics",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				if tt.skipReason != "" {
					t.Skip(tt.skipReason)
				}

				srv := newTestServer(t)
				putAndReadBack(t, srv, tt.object, tt.fixture, tt.wantData)
			})
		}
	})

	t.Run("vtodo", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name    string
			object  string
			fixture string
		}{
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/put.xml#L118-L143
				name:    "plain todo",
				object:  tasksPath(userOne) + "1todo.ics",
				fixture: "Resource/CalDAV/todo/1.txt",
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/put.xml#L144-L168
				name:    "due only",
				object:  tasksPath(userOne) + "2todo.ics",
				fixture: "Resource/CalDAV/todo/2.txt",
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/put.xml#L169-L193
				name:    "dtstart and duration",
				object:  tasksPath(userOne) + "3todo.ics",
				fixture: "Resource/CalDAV/todo/3.txt",
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/put.xml#L194-L218
				name:    "dtstart only",
				object:  tasksPath(userOne) + "4todo.ics",
				fixture: "Resource/CalDAV/todo/4.txt",
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/put.xml#L219-L243
				name:    "completed only",
				object:  tasksPath(userOne) + "5todo.ics",
				fixture: "Resource/CalDAV/todo/12.ics",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				srv := newTestServer(t)
				putAndReadBack(t, srv, tt.object, tt.fixture, tt.fixture)
			})
		}
	})

	t.Run("problem vevents", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name    string
			object  string
			fixture string
		}{
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/put.xml#L247-L260
				name:    "exdate combinations",
				object:  calendarPath(userOne) + object3,
				fixture: "Resource/CalDAV/problemics/1.ics",
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/put.xml#L261-L274
				name:    "dtstart utc dtend date",
				object:  calendarPath(userOne) + object4,
				fixture: "Resource/CalDAV/problemics/2.ics",
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/put.xml#L275-L288
				name:    "dtstart tzid dtend date",
				object:  calendarPath(userOne) + object18,
				fixture: "Resource/CalDAV/problemics/16.ics",
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/put.xml#L289-L302
				name:    "dtstart floating dtend date",
				object:  calendarPath(userOne) + object19,
				fixture: "Resource/CalDAV/problemics/17.ics",
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/put.xml#L303-L316
				name:    "dtstart utc dtend tzid",
				object:  calendarPath(userOne) + object5,
				fixture: "Resource/CalDAV/problemics/3.ics",
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/put.xml#L317-L330
				name:    "dtstart utc dtend floating",
				object:  calendarPath(userOne) + object6,
				fixture: "Resource/CalDAV/problemics/4.ics",
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/put.xml#L331-L344
				name:    "dtstart tzid dtend utc",
				object:  calendarPath(userOne) + object7,
				fixture: "Resource/CalDAV/problemics/5.ics",
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/put.xml#L345-L358
				name:    "dtstart tzid dtend floating",
				object:  calendarPath(userOne) + object8,
				fixture: "Resource/CalDAV/problemics/6.ics",
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/put.xml#L359-L372
				name:    "dtstart floating dtend utc",
				object:  calendarPath(userOne) + object9,
				fixture: "Resource/CalDAV/problemics/7.ics",
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/put.xml#L373-L386
				name:    "dtstart floating dtend tzid",
				object:  calendarPath(userOne) + object10,
				fixture: "Resource/CalDAV/problemics/8.ics",
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/put.xml#L387-L400
				name:    "recurrence id before master",
				object:  calendarPath(userOne) + "11-23.ics",
				fixture: "Resource/CalDAV/problemics/23.ics",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				srv := newTestServer(t)
				putAndReadBack(t, srv, tt.object, tt.fixture, tt.fixture)
			})
		}
	})

	t.Run("problem vtodos", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name    string
			object  string
			fixture string
		}{
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/put.xml#L403-L417
				name:    "dtstart utc due date",
				object:  tasksPath(userOne) + "11.ics",
				fixture: "Resource/CalDAV/problemics/9.ics",
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/put.xml#L418-L431
				name:    "dtstart tzid due date",
				object:  tasksPath(userOne) + "20.ics",
				fixture: "Resource/CalDAV/problemics/18.ics",
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/put.xml#L432-L445
				name:    "dtstart floating due date",
				object:  tasksPath(userOne) + "21.ics",
				fixture: "Resource/CalDAV/problemics/19.ics",
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/put.xml#L446-L459
				name:    "dtstart date due utc",
				object:  tasksPath(userOne) + "22.ics",
				fixture: "Resource/CalDAV/problemics/20.ics",
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/put.xml#L460-L473
				name:    "dtstart date due tzid",
				object:  tasksPath(userOne) + "23.ics",
				fixture: "Resource/CalDAV/problemics/21.ics",
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/put.xml#L474-L487
				name:    "dtstart date due floating",
				object:  tasksPath(userOne) + "24.ics",
				fixture: "Resource/CalDAV/problemics/22.ics",
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/put.xml#L488-L501
				name:    "dtstart utc due tzid",
				object:  tasksPath(userOne) + "12.ics",
				fixture: "Resource/CalDAV/problemics/10.ics",
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/put.xml#L502-L515
				name:    "dtstart utc due floating",
				object:  tasksPath(userOne) + "13.ics",
				fixture: "Resource/CalDAV/problemics/11.ics",
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/put.xml#L516-L529
				name:    "dtstart tzid due utc",
				object:  tasksPath(userOne) + "14.ics",
				fixture: "Resource/CalDAV/problemics/12.ics",
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/put.xml#L530-L543
				name:    "dtstart tzid due floating",
				object:  tasksPath(userOne) + "15.ics",
				fixture: "Resource/CalDAV/problemics/13.ics",
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/put.xml#L544-L557
				name:    "dtstart floating due utc",
				object:  tasksPath(userOne) + "16.ics",
				fixture: "Resource/CalDAV/problemics/14.ics",
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/put.xml#L558-L571
				name:    "dtstart floating due tzid",
				object:  tasksPath(userOne) + "17.ics",
				fixture: "Resource/CalDAV/problemics/15.ics",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				srv := newTestServer(t)
				putAndReadBack(t, srv, tt.object, tt.fixture, tt.fixture)
			})
		}
	})

	t.Run("content type parameters", func(t *testing.T) {
		t.Parallel()

		// Ported from:
		// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/put.xml#L575-L587
		srv := newTestServer(t)
		putAndReadBack(t, srv, calendarPath(userOne)+object1, fixtureRecurrenceput9, fixtureRecurrenceput9)
	})

	t.Run("content type parameters on get", func(t *testing.T) {
		t.Parallel()

		// Ported from:
		// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/put.xml#L589-L602
		t.Skip("Apple compatibility: exact Content-Type parameter formatting is not an RFC requirement.")

		srv := newTestServer(t)

		res := putAppleFixture(t, srv, userOne, calendarPath(userOne)+object1, fixtureRecurrenceput9)
		requireSuccess(t, res)

		res = doRequest(t, srv, http.MethodGet, calendarPath(userOne)+object1)
		requireSuccess(t, res)
		assert.Equal(t, "text/calendar;charset=utf-8", res.Header.Get("Content-Type"))
	})

	t.Run("content type parameters on propfind", func(t *testing.T) {
		t.Parallel()

		// Ported from:
		// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/put.xml#L603-L624
		t.Skip("Apple compatibility: exact Content-Type parameter formatting is not an RFC requirement.")

		srv := newTestServer(t)

		res := putAppleFixture(t, srv, userOne, calendarPath(userOne)+object1, fixtureRecurrenceput9)
		requireSuccess(t, res)

		res = propFind(
			t,
			srv,
			calendarPath(userOne)+object1,
			withDepth("0"),
			withBody(loadAppleFixture(t, "Resource/CalDAV/recurrenceput/10.xml", nil)),
		)
		require.Equal(t, http.StatusMultiStatus, res.Status, "body: %s", res.Body)
	})

	t.Run("regular collection content type", func(t *testing.T) {
		t.Parallel()

		// Ported from:
		// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/put.xml#L625-L696
		t.Skip("Optional: regular-collection is not planned.")
	})

	t.Run("rrule truncation", func(t *testing.T) {
		t.Parallel()

		// Ported from:
		// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/put.xml#L699-L892
		t.Skip("Apple compatibility: Apple marks RRULE truncation cases as ignored.")
	})

	t.Run("relaxed parsing", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name     string
			object   string
			fixture  string
			wantData string
		}{
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/put.xml#L896-L919
				name:     "invalid duration value",
				object:   calendarPath(userOne) + "i1.ics",
				fixture:  "Resource/CalDAV/put/1.ics",
				wantData: "Resource/CalDAV/put/2.ics",
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/put.xml#L920-L944
				name:     "backslash in uris",
				object:   calendarPath(userOne) + "i2.ics",
				fixture:  "Resource/CalDAV/put/4.ics",
				wantData: "Resource/CalDAV/put/5.ics",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				t.Skip(skipPermissiveParsing)

				srv := newTestServer(t)
				putAndReadBack(t, srv, tt.object, tt.fixture, tt.wantData)
			})
		}
	})

	t.Run("x parameter value", func(t *testing.T) {
		t.Parallel()

		// Ported from:
		// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/put.xml#L947-L973
		t.Skip("Apple compatibility: Apple-specific parameter normalization is not an RFC requirement.")

		srv := newTestServer(t)
		putAndReadBack(
			t,
			srv,
			calendarPath(userOne)+"3X.ics",
			"Resource/CalDAV/put/3.ics",
			"Resource/CalDAV/put/13.ics",
		)
	})

	t.Run("parameter encoding", func(t *testing.T) {
		t.Parallel()

		// Ported from:
		// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/put.xml#L975-L1001
		t.Skip("Apple compatibility: Apple-specific parameter encoding normalization is not an RFC requirement.")

		srv := newTestServer(t)
		putAndReadBack(
			t,
			srv,
			calendarPath(userOne)+"106.ics",
			"Resource/CalDAV/put/6.ics",
			"Resource/CalDAV/put/7.ics",
		)
	})

	t.Run("duplicate alarms", func(t *testing.T) {
		t.Parallel()

		// Ported from:
		// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/put.xml#L1003-L1057
		t.Skip(skipDuplicateAlarms)
	})
}
