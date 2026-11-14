package caldavtest

import (
	"net/http"
	"testing"
)

func TestAppleRecurrencePut(t *testing.T) {
	t.Parallel()

	// Expected readback data comes from each case fixture under Resource/CalDAV/recurrenceput.

	t.Run("vevents", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name     string
			object   string
			fixture  string
			wantData string
		}{
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/recurrenceput.xml#L32-L45
				name:    "non-recurring timed",
				object:  calendarPath(userOne) + object1,
				fixture: fixtureRecurrenceput1,
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/recurrenceput.xml#L46-L59
				name:    "non-recurring all day",
				object:  calendarPath(userOne) + object2,
				fixture: "Resource/CalDAV/recurrenceput/2.txt",
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/recurrenceput.xml#L60-L73
				name:    "recurring timed",
				object:  calendarPath(userOne) + object3,
				fixture: "Resource/CalDAV/recurrenceput/3.txt",
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/recurrenceput.xml#L74-L87
				name:    "recurring all day",
				object:  calendarPath(userOne) + object4,
				fixture: "Resource/CalDAV/recurrenceput/4.txt",
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/recurrenceput.xml#L88-L101
				name:    "recurring with exceptions timed",
				object:  calendarPath(userOne) + object5,
				fixture: "Resource/CalDAV/recurrenceput/5.txt",
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/recurrenceput.xml#L102-L115
				name:    "recurring with exceptions all day",
				object:  calendarPath(userOne) + object6,
				fixture: "Resource/CalDAV/recurrenceput/6.txt",
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/recurrenceput.xml#L116-L129
				name:    "recurring with future exceptions timed",
				object:  calendarPath(userOne) + object7,
				fixture: "Resource/CalDAV/recurrenceput/7.txt",
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/recurrenceput.xml#L130-L143
				name:    "recurring with future exceptions all day",
				object:  calendarPath(userOne) + object8,
				fixture: "Resource/CalDAV/recurrenceput/8.txt",
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/recurrenceput.xml#L144-L164
				name:    "two exceptions one year apart",
				object:  calendarPath(userOne) + object9,
				fixture: "Resource/CalDAV/recurrenceput/22.ics",
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/recurrenceput.xml#L165-L185
				// Apple repeats the same request for cases 10 and 11. These rows provide no independent coverage.
				name:    "invalid override and master cancel",
				object:  calendarPath(userOne) + object10,
				fixture: "Resource/CalDAV/recurrenceput/23.ics",
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/recurrenceput.xml#L186-L206
				// Apple repeats the same request for cases 10 and 11. These rows provide no independent coverage.
				name:    "invalid cancelled override and master",
				object:  calendarPath(userOne) + object10,
				fixture: "Resource/CalDAV/recurrenceput/23.ics",
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/recurrenceput.xml#L207-L231
				name:     "monthly byday bysetpos with override",
				object:   calendarPath(userOne) + object11,
				fixture:  "Resource/CalDAV/recurrenceput/25.ics",
				wantData: "Resource/CalDAV/recurrenceput/25.ics",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				srv := newTestServer(t)

				res := putAppleFixture(t, srv, userOne, tt.object, tt.fixture)
				requireSuccess(t, res)

				res = doRequest(t, srv, http.MethodGet, tt.object)
				requireSuccess(t, res)

				want := tt.wantData
				if want == "" {
					want = tt.fixture
				}

				assertCalendarDataEqual(t, res.Body, loadAppleFixture(t, want, nil))
			})
		}
	})

	t.Run("vtodos", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name    string
			object  string
			fixture string
		}{
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/recurrenceput.xml#L235-L248
				name:    "plain todo",
				object:  tasksPath(userOne) + "1todo.ics",
				fixture: "Resource/CalDAV/todo/5.txt",
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/recurrenceput.xml#L249-L262
				name:    "all day due",
				object:  tasksPath(userOne) + "2todo.ics",
				fixture: "Resource/CalDAV/todo/6.txt",
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/recurrenceput.xml#L263-L276
				name:    "all day start and duration",
				object:  tasksPath(userOne) + "3todo.ics",
				fixture: "Resource/CalDAV/todo/7.txt",
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/recurrenceput.xml#L277-L290
				name:    "all day start only",
				object:  tasksPath(userOne) + "3todo.ics",
				fixture: "Resource/CalDAV/todo/8.txt",
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/recurrenceput.xml#L291-L304
				name:    "timed due only",
				object:  tasksPath(userOne) + "4todo.ics",
				fixture: "Resource/CalDAV/todo/9.txt",
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/recurrenceput.xml#L305-L318
				name:    "timed start and duration",
				object:  tasksPath(userOne) + "5todo.ics",
				fixture: "Resource/CalDAV/todo/10.txt",
			},
			{
				// Ported from:
				// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/recurrenceput.xml#L319-L332
				name:    "timed start and duration repeated",
				object:  tasksPath(userOne) + "5todo.ics",
				fixture: "Resource/CalDAV/todo/11.txt",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				srv := newTestServer(t)

				res := putAppleFixture(t, srv, userOne, tt.object, tt.fixture)
				requireSuccess(t, res)
			})
		}
	})
}
