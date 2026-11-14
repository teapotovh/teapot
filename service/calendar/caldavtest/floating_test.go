package caldavtest

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/teapotovh/teapot/lib/webdav"
)

func newFloatingServer(t *testing.T) *httptest.Server {
	t.Helper()

	// Ported from:
	// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/floating.xml#L28-L153
	srv := newTestServer(t)

	nonePath := calendarHome(userOne) + "calendar-none/"
	res := createCalendar(t, srv, userOne, nonePath)
	require.Equal(t, http.StatusCreated, res.Status, "body: %s", res.Body)

	for i := 1; i <= 7; i++ {
		res = putAppleFixture(
			t,
			srv,
			userOne,
			nonePath+strconv.Itoa(i)+".ics",
			"Resource/CalDAV/floating/put/"+strconv.Itoa(i)+".txt",
		)
		requireSuccess(t, res)
	}

	usPath := calendarHome(userOne) + "calendar-us/"
	res = doRequest(
		t,
		srv,
		webdav.MethodMkcalendar,
		usPath,
		withContentType(xmlContentType),
		withBody(loadAppleFixture(t, "Resource/CalDAV/floating/put/8.txt", nil)),
	)
	require.Equal(t, http.StatusCreated, res.Status, "body: %s", res.Body)

	for i := 1; i <= 7; i++ {
		res = putAppleFixture(
			t,
			srv,
			userOne,
			usPath+strconv.Itoa(i)+".ics",
			"Resource/CalDAV/floating/put/"+strconv.Itoa(i+8)+".ics",
		)
		requireSuccess(t, res)
	}

	return srv
}

func TestAppleFloating(t *testing.T) {
	t.Parallel()

	t.Run("report without calendar timezone", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name    string
			fixture string
			want    []string
			reason  string
		}{
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/floating.xml#L156-L180
				name:    "us eastern",
				fixture: fixtureFloating1,
				want:    []string{"1.ics", "3.ics", "5.ics", "6.ics"},
				reason:  "RFC 4791 7.8.4: floating-time and calendar-timezone query semantics are not implemented.",
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/floating.xml#L181-L204
				name:    "us pacific",
				fixture: fixtureFloating2,
				want:    []string{"1.ics", "3.ics", "6.ics"},
				reason:  "RFC 4791 7.8.4: floating-time and calendar-timezone query semantics are not implemented.",
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/floating.xml#L205-L230
				name:    "gmt",
				fixture: "Resource/CalDAV/floating/3.txt",
				want:    []string{"1.ics", "3.ics", "5.ics", "6.ics", "7.ics"},
				reason:  "RFC 4791 7.8.4: floating-time and calendar-timezone query semantics are not implemented.",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				t.Skip(tt.reason)

				srv := newFloatingServer(t)
				path := calendarHome(userOne) + "calendar-none/"

				res := report(t, srv, path, loadAppleFixture(t, tt.fixture, nil))
				assertHrefBases(t, res, tt.want)
			})
		}
	})

	t.Run("report with calendar timezone", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name    string
			fixture string
			want    []string
		}{
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/floating.xml#L234-L258
				name:    "no timezone",
				fixture: fixtureFloating1,
				want:    []string{"1.ics", "3.ics", "5.ics", "6.ics"},
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/floating.xml#L259-L283
				name:    "us eastern",
				fixture: fixtureFloating1,
				want:    []string{"1.ics", "3.ics", "5.ics", "6.ics"},
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/floating.xml#L284-L307
				name:    "us pacific",
				fixture: fixtureFloating2,
				want:    []string{"1.ics", "3.ics", "6.ics"},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				t.Skip("RFC 4791 7.8.4: calendar-timezone property semantics are not implemented.")

				srv := newFloatingServer(t)
				path := calendarHome(userOne) + "calendar-us/"

				res := report(t, srv, path, loadAppleFixture(t, tt.fixture, nil))
				assertHrefBases(t, res, tt.want)
			})
		}
	})

	t.Run("report on collection", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name    string
			fixture string
			want    []string
		}{
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/floating.xml#L311-L343
				name:    "no timezone",
				fixture: fixtureFloating1,
				want: []string{
					"calendar-none/1.ics", "calendar-none/3.ics", "calendar-none/5.ics",
					"calendar-none/6.ics", "calendar-us/1.ics", "calendar-us/3.ics",
					"calendar-us/5.ics", "calendar-us/6.ics",
				},
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/floating.xml#L344-L376
				name:    "us eastern",
				fixture: fixtureFloating1,
				want: []string{
					"calendar-none/1.ics", "calendar-none/3.ics", "calendar-none/5.ics",
					"calendar-none/6.ics", "calendar-us/1.ics", "calendar-us/3.ics",
					"calendar-us/5.ics", "calendar-us/6.ics",
				},
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/floating.xml#L377-L407
				name:    "us pacific",
				fixture: fixtureFloating2,
				want: []string{
					"calendar-none/1.ics", "calendar-none/3.ics", "calendar-none/6.ics",
					"calendar-us/1.ics", "calendar-us/3.ics", "calendar-us/6.ics",
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				t.Skip("Optional: REPORT on a calendar home collection is not planned.")

				srv := newFloatingServer(t)

				res := report(
					t,
					srv,
					calendarHome(userOne),
					loadAppleFixture(t, tt.fixture, nil),
					withDepth("1"),
				)

				hrefs := make([]string, len(tt.want))
				for i, want := range tt.want {
					hrefs[i] = calendarHome(userOne) + want
				}

				assertOKHrefs(t, res, hrefs)
			})
		}
	})

	t.Run("report expand", func(t *testing.T) {
		t.Parallel()

		// Ported from:
		// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/floating.xml#L410-L441
		t.Skip("RFC 4791 9.6.5: calendar-data expand is not implemented.")

		srv := newFloatingServer(t)

		res := report(
			t,
			srv,
			calendarHome(userOne),
			loadAppleFixture(t, "Resource/CalDAV/floating/6.xml", nil),
			withDepth("1"),
		)

		home := calendarHome(userOne)
		assertOKHrefs(t, res, []string{
			home + "calendar-none/1.ics",
			home + "calendar-none/2.ics",
			home + "calendar-none/4.ics",
			home + "calendar-us/1.ics",
			home + "calendar-us/2.ics",
			home + "calendar-us/4.ics",
		})
	})

	t.Run("free busy", func(t *testing.T) {
		t.Parallel()

		// Ported from:
		// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/floating.xml#L444-L525
		tests := []struct {
			name    string
			path    string
			busy    []string
			tent    []string
			unavail []string
		}{
			{
				name: "collection without timezone",
				path: calendarHome(userOne) + "calendar-none/",
				busy: []string{
					"0101T000000Z/0101T020000Z",
					"0101T050000Z/0101T060000Z",
					"0101T080000Z/0101T090000Z",
				},
				tent: []string{"0101T000000Z/0102T000000Z"},
			},
			{
				name: "collection with timezone",
				path: calendarHome(userOne) + "calendar-us/",
				busy: []string{
					"0101T000000Z/0101T010000Z",
					"0101T050000Z/0101T070000Z",
					"0101T080000Z/0101T090000Z",
				},
				tent: []string{"0101T050000Z/0102T000000Z"},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				t.Skip(skipFreeBusy)

				srv := newFloatingServer(t)

				res := report(
					t,
					srv,
					tt.path,
					loadAppleFixture(t, "Resource/CalDAV/floating/5.txt", nil),
					withDepth("1"),
				)
				require.Equal(t, http.StatusOK, res.Status, "body: %s", res.Body)
				assertFreeBusy(t, res.Body, tt.busy, tt.tent, tt.unavail)
			})
		}
	})

	t.Run("floating dtstart with rrule until", func(t *testing.T) {
		t.Parallel()

		// Ported from:
		// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/floating.xml#L527-L542
		srv := newTestServer(t)

		res := putAppleFixture(t, srv, userOne, calendarPath(userOne)+object7, "Resource/CalDAV/floating/7.ics")
		requireSuccess(t, res)

		res = doRequest(t, srv, http.MethodGet, calendarPath(userOne)+object7)
		requireSuccess(t, res)
		assertCalendarDataEqual(t, res.Body, loadAppleFixture(t, "Resource/CalDAV/floating/7.ics", nil))
	})
}
