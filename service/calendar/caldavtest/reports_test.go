package caldavtest

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type reportPut struct {
	object  string
	fixture string
}

const reportCollectionModel = "non-split"

type reportHrefSet struct {
	model string
	ok    []string
	bad   []string
}

type reportCase struct {
	name         string
	path         string
	fixture      string
	okHrefs      []string
	badHrefs     []string
	hrefSets     []reportHrefSet
	status       int
	precondition string
	contains     []string
	notContains  []string
	puts         []reportPut
	skipReason   string
}

func newReportsServer(t *testing.T) *httptest.Server {
	t.Helper()

	// Ported from:
	// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L26-L246
	srv := newTestServer(t)

	for _, name := range []string{
		"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15",
		"18", "19", "20", "21", "22",
	} {
		res := putAppleFixture(
			t,
			srv,
			userOne,
			calendarPath(userOne)+name+".ics",
			"Resource/CalDAV/reports/put/"+name+".txt",
		)
		requireSuccess(t, res)
	}

	res := putAppleFixture(t, srv, userOne, calendarPath(userOne)+object27, "Resource/CalDAV/reports/put/27.ics")
	requireSuccess(t, res)

	for _, name := range []string{"101", "102", "103", "104", "105", "106"} {
		res := putAppleFixture(
			t,
			srv,
			userOne,
			tasksPath(userOne)+name+".ics",
			"Resource/CalDAV/reports/put/"+name+".txt",
		)
		requireSuccess(t, res)
	}

	return srv
}

func runReportCases(t *testing.T, tests []reportCase) {
	t.Helper()

	// Active Apple REPORT verifiers use multistatusItems. The exact href and status sets are the checked result.
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.skipReason != "" {
				t.Skip(tt.skipReason)
			}

			srv := newReportsServer(t)

			for _, put := range tt.puts {
				res := putAppleFixture(t, srv, userOne, put.object, put.fixture)
				requireSuccess(t, res)
			}

			res := report(t, srv, tt.path, loadAppleFixture(t, tt.fixture, nil))

			expectedOK := tt.okHrefs

			expectedBad := tt.badHrefs
			for _, set := range tt.hrefSets {
				if set.model == reportCollectionModel {
					expectedOK = set.ok
					expectedBad = set.bad

					break
				}
			}

			okHrefs := make([]string, len(expectedOK))
			for i, href := range expectedOK {
				okHrefs[i] = tt.path + href
			}

			badHrefs := make([]string, len(expectedBad))
			for i, href := range expectedBad {
				badHrefs[i] = tt.path + href
			}

			switch {
			case tt.status != 0:
				require.Equal(t, tt.status, res.Status, "body: %s", res.Body)
			case tt.precondition != "":
				require.NotEqual(t, http.StatusMultiStatus, res.Status, "body: %s", res.Body)
				davErr := parseDAVError(t, res.Body)
				assert.True(t, hasName(davErr.Names, caldavNamespace, tt.precondition), "body: %s", res.Body)
			default:
				assertReportHrefs(t, res, okHrefs, badHrefs)

				if len(okHrefs) > 0 {
					assertAllResponsesHaveProps(
						t,
						res.Body,
						http.StatusOK,
						xml.Name{Space: caldavNamespace, Local: "calendar-data"},
					)
				}
			}

			for _, value := range tt.contains {
				assert.Contains(t, res.Body, expandAppleValue(value, nil), "body: %s", res.Body)
			}

			for _, value := range tt.notContains {
				assert.NotContains(t, res.Body, expandAppleValue(value, nil), "body: %s", res.Body)
			}
		})
	}
}

func TestAppleReportsMultiget(t *testing.T) {
	t.Parallel()

	tests := []reportCase{
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L271-L311
			name:    "multiget 1a",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/multiget/1.txt",
			okHrefs: []string{object1, object2, object3, object4, object101, object102},
			hrefSets: []reportHrefSet{
				{model: "non-split", ok: []string{object1, object2, object3, object4, object101, object102}, bad: nil},
				{model: "split", ok: []string{object1, object2, object3, object4}, bad: []string{object101, object102}},
			},
			skipReason: "Apple compatibility: single-collection model with tasks in the calendar is not implemented.",
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L318-L340
			name:       "multiget 1b",
			path:       tasksPath(userOne),
			fixture:    "Resource/CalDAV/reports/multiget/1b.xml",
			badHrefs:   []string{object101, object102},
			skipReason: skipSplitCalendars,
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L344-L361
			name:    "multiget 2",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/multiget/2.txt",
			okHrefs: []string{object1, object2, object3, object4},
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L365-L382
			name:    "multiget 3",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/multiget/3.txt",
			okHrefs: []string{object1, object2, object3, object4},
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L386-L426
			name:    "multiget 4a",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/multiget/4.txt",
			okHrefs: []string{object1, object2, object3, object4, object101, object102},
			hrefSets: []reportHrefSet{
				{model: "non-split", ok: []string{object1, object2, object3, object4, object101, object102}, bad: nil},
				{model: "split", ok: []string{object1, object2, object3, object4}, bad: []string{object101, object102}},
			},
			skipReason: "Apple compatibility: single-collection model with tasks in the calendar is not implemented.",
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L433-L455
			name:       "multiget 4b",
			path:       tasksPath(userOne),
			fixture:    "Resource/CalDAV/reports/multiget/4b.xml",
			badHrefs:   []string{object101, object102},
			skipReason: skipSplitCalendars,
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L459-L499
			name:    "multiget 5a",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/multiget/5.txt",
			okHrefs: []string{object1, object2, object3, object4, object101, object102},
			hrefSets: []reportHrefSet{
				{model: "non-split", ok: []string{object1, object2, object3, object4, object101, object102}, bad: nil},
				{model: "split", ok: []string{object1, object2, object3, object4}, bad: []string{object101, object102}},
			},
			skipReason: "Apple compatibility: single-collection model with tasks in the calendar is not implemented.",
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L506-L528
			name:       "multiget 5b",
			path:       tasksPath(userOne),
			fixture:    "Resource/CalDAV/reports/multiget/5b.xml",
			badHrefs:   []string{object101, object102},
			skipReason: skipSplitCalendars,
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L532-L549
			name:    "multiget 6",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/multiget/6.txt",
			okHrefs: []string{object1, object2, object3, object4},
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L553-L570
			name:    "multiget 7",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/multiget/7.txt",
			okHrefs: []string{object1, object2, object3, object4},
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L574-L595
			name:     "multiget 8",
			path:     calendarPath(userOne),
			fixture:  "Resource/CalDAV/reports/multiget/8.xml",
			badHrefs: []string{"bogus-resource.ics"},
		},
	}

	runReportCases(t, tests)
}

func TestAppleReportsBasicQuery(t *testing.T) {
	t.Parallel()

	tests := []reportCase{
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L602-L635
			name:    "basic query 1a",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/basicquery/1.txt",
			okHrefs: []string{
				object1,
				object2,
				object3,
				object4,
				object5,
				object6,
				object7,
				object8,
				object9,
				object10,
				object11,
				object12,
				object13,
				object14,
				object18,
				object19,
				object20,
				object21,
				object22,
				object27,
			},
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L642-L655
			name:       "basic query 1b",
			path:       tasksPath(userOne),
			fixture:    "Resource/CalDAV/reports/basicquery/1.txt",
			skipReason: skipSplitCalendars,
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L659-L691
			name:    "basic query 2a",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/basicquery/8.txt",
			okHrefs: []string{object15, object101, object102, object103, object104, object105, object106},
			hrefSets: []reportHrefSet{
				{
					model: "non-split",
					ok:    []string{object15, object101, object102, object103, object104, object105, object106},
				},
				{model: "split", ok: nil, bad: nil},
			},
			skipReason: "Apple compatibility: single-collection model with tasks in the calendar is not implemented.",
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L698-L717
			name:       "basic query 2b",
			path:       tasksPath(userOne),
			fixture:    "Resource/CalDAV/reports/basicquery/8.txt",
			okHrefs:    []string{object101, object102, object103, object104, object105, object106},
			skipReason: skipSplitCalendars,
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L721-L743
			name:    "basic query 3",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/basicquery/2.txt",
			okHrefs: []string{object1, object10, object11, object12, object13, object14, object18, object20, object21},
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L747-L771
			name:    "basic query 4",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/basicquery/7.txt",
			okHrefs: []string{
				object2,
				object3,
				object4,
				object5,
				object6,
				object7,
				object8,
				object9,
				object19,
				object22,
				object27,
			},
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L775-L791
			name:    "basic query 5",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/basicquery/3.txt",
			okHrefs: []string{object2, object6, object7},
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L795-L827
			name:    "basic query 6",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/basicquery/9.txt",
			okHrefs: []string{
				object1,
				object3,
				object4,
				object5,
				object6,
				object7,
				object8,
				object9,
				object10,
				object11,
				object12,
				object13,
				object14,
				object18,
				object19,
				object20,
				object21,
				object22,
				object27,
			},
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L831-L845
			name:    "basic query 7",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/basicquery/4.txt",
			okHrefs: []string{object3},
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L849-L879
			name:    "basic query 8",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/basicquery/10.txt",
			okHrefs: []string{
				object1,
				object2,
				object4,
				object5,
				object6,
				object7,
				object8,
				object9,
				object10,
				object11,
				object12,
				object13,
				object14,
				object19,
				object20,
				object21,
				object22,
			},
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L883-L896
			name:    "basic query 9",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/basicquery/5.txt",
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L903-L918
			name:       "basic query 10",
			path:       calendarPath(userOne),
			fixture:    "Resource/CalDAV/reports/basicquery/16.xml",
			okHrefs:    []string{object2, object3},
			skipReason: skipQueryExtended,
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L922-L937
			name:    "basic query 11",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/basicquery/6.txt",
			okHrefs: []string{object12, object22},
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L944-L975
			name:    "basic query 12",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/basicquery/17.xml",
			okHrefs: []string{
				object1,
				object2,
				object4,
				object5,
				object6,
				object7,
				object8,
				object9,
				object10,
				object11,
				object12,
				object13,
				object14,
				object19,
				object20,
				object21,
				object22,
				object27,
			},
			skipReason: skipQueryExtended,
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L979-L994
			name:    "basic query 13",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/basicquery/11.txt",
			okHrefs: []string{object6, object22},
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L998-L1029
			name:    "basic query 14a",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/basicquery/101.txt",
			okHrefs: []string{object101, object102, object103, object104, object105, object106},
			hrefSets: []reportHrefSet{
				{
					model: "non-split",
					ok:    []string{object101, object102, object103, object104, object105, object106},
				},
				{model: "split", ok: nil, bad: nil},
			},
			skipReason: "Apple compatibility: single-collection model with tasks in the calendar is not implemented.",
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L1036-L1055
			name:       "basic query 14b",
			path:       tasksPath(userOne),
			fixture:    "Resource/CalDAV/reports/basicquery/101.txt",
			okHrefs:    []string{object101, object102, object103, object104, object105, object106},
			skipReason: skipSplitCalendars,
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L1059-L1073
			name:    "basic query 15",
			path:    tasksPath(userOne),
			fixture: "Resource/CalDAV/reports/basicquery/102.txt",
			okHrefs: []string{object101},
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L1077-L1093
			name:    "basic query 16",
			path:    tasksPath(userOne),
			fixture: "Resource/CalDAV/reports/basicquery/103.txt",
			okHrefs: []string{object102, object104, object106},
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L1097-L1110
			name:    "basic query 17a",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/basicquery/201.txt",
			okHrefs: []string{},
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L1117-L1130
			name:       "basic query 17b",
			path:       tasksPath(userOne),
			fixture:    "Resource/CalDAV/reports/basicquery/201.txt",
			skipReason: skipSplitCalendars,
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L1137-L1207
			name:    "basic query 18a",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/basicquery/204.xml",
			okHrefs: []string{
				object1,
				object2,
				object3,
				object4,
				object5,
				object6,
				object7,
				object8,
				object9,
				object10,
				object11,
				object12,
				object13,
				object14,
				object18,
				object19,
				object20,
				object21,
				object22,
				object101,
				object102,
				object103,
				object104,
				object105,
				object106,
			},
			skipReason: skipQueryExtended,
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L1215-L1234
			name:       "basic query 18b",
			path:       tasksPath(userOne),
			fixture:    "Resource/CalDAV/reports/basicquery/204.xml",
			okHrefs:    []string{object101, object102, object103, object104, object105, object106},
			skipReason: skipQueryExtended,
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L1238-L1251
			name:    "basic query 19a",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/basicquery/202.txt",
			okHrefs: []string{},
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L1258-L1271
			name:       "basic query 19b",
			path:       tasksPath(userOne),
			fixture:    "Resource/CalDAV/reports/basicquery/202.txt",
			skipReason: skipSplitCalendars,
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L1278-L1322
			name:    "basic query 20a",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/basicquery/205.xml",
			okHrefs: []string{
				object1,
				object10,
				object11,
				object12,
				object13,
				object14,
				object18,
				object20,
				object21,
				object101,
			},
			skipReason: skipQueryExtended,
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L1330-L1344
			name:       "basic query 20b",
			path:       tasksPath(userOne),
			fixture:    "Resource/CalDAV/reports/basicquery/205.xml",
			okHrefs:    []string{object101},
			skipReason: skipQueryExtended,
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L1348-L1361
			name:    "basic query 21",
			path:    tasksPath(userOne),
			fixture: "Resource/CalDAV/reports/basicquery/203.txt",
			okHrefs: []string{},
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L1368-L1398
			name:       "basic query 22a",
			path:       calendarPath(userOne),
			fixture:    "Resource/CalDAV/reports/basicquery/206.xml",
			okHrefs:    []string{object3, object102, object104, object106},
			skipReason: skipQueryExtended,
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L1406-L1422
			name:       "basic query 22b",
			path:       tasksPath(userOne),
			fixture:    "Resource/CalDAV/reports/basicquery/206.xml",
			okHrefs:    []string{object102, object104, object106},
			skipReason: skipQueryExtended,
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L1426-L1456
			name:    "basic query 23",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/basicquery/12.txt",
			okHrefs: []string{
				object1,
				object2,
				object3,
				object6,
				object8,
				object9,
				object10,
				object11,
				object12,
				object13,
				object14,
				object18,
				object19,
				object20,
				object21,
				object22,
				object27,
			},
			skipReason: "RFC 4791 9.7.2: comp-filter is-not-defined inside VEVENT must exclude events with VALARM.",
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L1460-L1478
			name:    "basic query 24",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/basicquery/13.txt",
			okHrefs: []string{object1, object2, object4, object5, object6},
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L1482-L1500
			name:       "basic query 25",
			path:       calendarPath(userOne),
			fixture:    "Resource/CalDAV/reports/basicquery/14.txt",
			okHrefs:    []string{object1, object2, object4, object5, object6},
			skipReason: "Optional: text-match collation is not planned.",
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L1504-L1514
			name:       "basic query 26",
			path:       calendarPath(userOne),
			fixture:    "Resource/CalDAV/reports/basicquery/15.xml",
			skipReason: "RFC 4791 9.7.1: a calendar-query must contain a VCALENDAR comp-filter; an empty filter must not match all objects.",
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L1521-L1535
			name:       "basic query 27",
			path:       calendarPath(userOne),
			fixture:    "Resource/CalDAV/reports/basicquery/18.xml",
			okHrefs:    []string{object4},
			skipReason: skipQueryExtended,
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L1542-L1556
			name:       "basic query 28",
			path:       calendarPath(userOne),
			fixture:    "Resource/CalDAV/reports/basicquery/19.xml",
			okHrefs:    []string{object14},
			skipReason: skipQueryExtended,
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L1563-L1577
			name:       "basic query 29",
			path:       calendarPath(userOne),
			fixture:    "Resource/CalDAV/reports/basicquery/20.xml",
			okHrefs:    []string{object1},
			skipReason: skipQueryExtended,
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L1584-L1604
			name:       "basic query 30",
			path:       calendarPath(userOne),
			fixture:    "Resource/CalDAV/reports/basicquery/21.xml",
			okHrefs:    []string{object10, object11, object12, object13, object14, object20, object21},
			skipReason: skipQueryExtended,
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L1611-L1633
			name:    "basic query 31",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/basicquery/22.xml",
			okHrefs: []string{
				object1,
				object10,
				object11,
				object12,
				object13,
				object14,
				object18,
				object20,
				object21,
			},
			skipReason: skipQueryExtended,
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L1640-L1656
			name:       "basic query 32",
			path:       calendarPath(userOne),
			fixture:    "Resource/CalDAV/reports/basicquery/23.xml",
			okHrefs:    []string{object1, object11, object18},
			skipReason: skipQueryExtended,
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L1663-L1677
			name:       "basic query 33",
			path:       calendarPath(userOne),
			fixture:    "Resource/CalDAV/reports/basicquery/24.xml",
			okHrefs:    []string{object2},
			skipReason: skipQueryExtended,
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L1684-L1700
			name:       "basic query 34",
			path:       calendarPath(userOne),
			fixture:    "Resource/CalDAV/reports/basicquery/25.xml",
			okHrefs:    []string{object2, object22, object27},
			skipReason: skipQueryExtended,
		},
	}

	runReportCases(t, tests)
}

func TestAppleReportsFilteredData(t *testing.T) {
	t.Parallel()

	tests := []reportCase{
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L1707-L1728
			name:        "filtered data 1",
			path:        calendarPath(userOne),
			fixture:     "Resource/CalDAV/reports/filtereddata/1.xml",
			okHrefs:     []string{object1},
			notContains: []string{"DTSTAMP"},
			skipReason:  "RFC 4791 9.6: partial calendar-data filtering is not implemented.",
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L1732-L1753
			name:        "filtered data 2",
			path:        calendarPath(userOne),
			fixture:     "Resource/CalDAV/reports/filtereddata/2.xml",
			okHrefs:     []string{object1},
			notContains: []string{"DTSTAMP"},
			skipReason:  "RFC 4791 9.6: partial calendar-data filtering is not implemented.",
		},
	}

	runReportCases(t, tests)
}

func TestAppleReportsTimeRange(t *testing.T) {
	t.Parallel()

	tests := []reportCase{
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L1760-L1777
			name:    "time range 1",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/timerangequery/1.txt",
			okHrefs: []string{object5, object6, object7, object19},
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L1781-L1794
			name:       "time range 2",
			path:       calendarPath(userOne),
			fixture:    "Resource/CalDAV/reports/timerangequery/2.txt",
			skipReason: "RFC 4791 9.9: time-range filters on VFREEBUSY are not implemented.",
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L1801-L1849
			name:    "time range 3",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/timerangequery/14.xml",
			okHrefs: []string{
				object1,
				object2,
				object3,
				object4,
				object8,
				object9,
				object10,
				object11,
				object12,
				object13,
				object14,
				object15,
			},
			skipReason: skipQueryExtended,
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L1853-L1867
			name:    "time range 4",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/timerangequery/3.txt",
			okHrefs: []string{object1},
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L1871-L1885
			name:    "time range 5",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/timerangequery/4.txt",
			okHrefs: []string{object6},
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L1889-L1903
			name:    "time range 6",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/timerangequery/5.txt",
			okHrefs: []string{object6},
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L1907-L1917
			name:    "time range 7",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/timerangequery/6.txt",
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L1921-L1935
			name:       "time range 8",
			path:       calendarPath(userOne),
			fixture:    "Resource/CalDAV/reports/timerangequery/7.txt",
			okHrefs:    []string{object7},
			skipReason: "RFC 5545 3.8.4.4: RANGE=THISANDFUTURE recurrence handling is not implemented.",
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L1939-L1955
			name:    "time range 9",
			path:    tasksPath(userOne),
			fixture: "Resource/CalDAV/reports/timerangequery/101.txt",
			okHrefs: []string{object101, object102, object103},
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L1959-L1973
			name:    "time range 10",
			path:    tasksPath(userOne),
			fixture: "Resource/CalDAV/reports/timerangequery/102.txt",
			okHrefs: []string{object102},
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L1977-L1993
			name:    "time range 11",
			path:    tasksPath(userOne),
			fixture: "Resource/CalDAV/reports/timerangequery/103.txt",
			okHrefs: []string{object101, object105, object106},
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L1997-L2011
			name:         "time range 12a",
			path:         calendarPath(userOne),
			fixture:      "Resource/CalDAV/reports/timerangequery/104.xml",
			precondition: "valid-calendar-data",
			skipReason:   "RFC 4791 5.3.2.1: invalid VTIMEZONE data must be rejected with valid-calendar-data.",
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L2018-L2032
			name:         "time range 12b",
			path:         calendarPath(userOne),
			fixture:      "Resource/CalDAV/reports/timerangequery/105.xml",
			precondition: "valid-timezone",
			skipReason:   "Optional: timezones-by-reference is not planned.",
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L2036-L2053
			name:    "time range 13",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/timerangequery/8.txt",
			okHrefs: []string{object19, object20, object21, object22},
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L2057-L2086
			name:    "time range 14",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/timerangequery/9.xml",
			okHrefs: []string{
				object5,
				object6,
				object7,
				object8,
				object9,
				object10,
				object11,
				object12,
				object13,
				object14,
				object18,
				object19,
				object20,
				object21,
				object22,
				object27,
			},
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L2090-L2111
			name:    "time range 15",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/timerangequery/10.xml",
			okHrefs: []string{object1, object2, object3, object4, object5, object6, object7, object19},
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L2115-L2129
			name:       "time range 16",
			path:       calendarPath(userOne),
			fixture:    "Resource/CalDAV/reports/timerangequery/11.xml",
			status:     400,
			skipReason: "RFC 4791 9.9: a time-range must have a start or an end.",
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L2133-L2147
			name:         "time range 17",
			path:         calendarPath(userOne),
			fixture:      "Resource/CalDAV/reports/timerangequery/12.xml",
			precondition: "number-of-recurrences-within-limits",
			skipReason:   "Apple compatibility: Apple marks this case as ignored.",
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L2162-L2176
			name:    "time range 18",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/timerangequery/13.xml",
			okHrefs: []string{object23},
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L2151-L2161
			puts: []reportPut{{
				object:  calendarPath(userOne) + object23,
				fixture: "Resource/CalDAV/reports/put/23.ics",
			}},
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L2180-L2193
			name:    "time range 19a",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/timerangequery/201.xml",
			okHrefs: []string{},
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L2200-L2213
			name:       "time range 19b",
			path:       tasksPath(userOne),
			fixture:    "Resource/CalDAV/reports/timerangequery/201.xml",
			skipReason: skipSplitCalendars,
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L2220-L2256
			name:       "time range 20a",
			path:       calendarPath(userOne),
			fixture:    "Resource/CalDAV/reports/timerangequery/204.xml",
			okHrefs:    []string{object5, object6, object7, object19, object101, object102, object103},
			skipReason: skipQueryExtended,
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L2264-L2280
			name:       "time range 20b",
			path:       tasksPath(userOne),
			fixture:    "Resource/CalDAV/reports/timerangequery/204.xml",
			okHrefs:    []string{object101, object102, object103},
			skipReason: skipQueryExtended,
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L2284-L2297
			name:    "time range 21a",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/timerangequery/202.xml",
			okHrefs: []string{},
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L2304-L2317
			name:       "time range 21b",
			path:       tasksPath(userOne),
			fixture:    "Resource/CalDAV/reports/timerangequery/202.xml",
			skipReason: skipSplitCalendars,
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L2324-L2387
			name:    "time range 22a",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/timerangequery/205.xml",
			okHrefs: []string{
				object5,
				object6,
				object7,
				object8,
				object9,
				object10,
				object11,
				object12,
				object13,
				object14,
				object18,
				object19,
				object20,
				object21,
				object22,
				object23,
				object27,
				object101,
				object104,
				object105,
				object106,
			},
			skipReason: skipQueryExtended,
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L2395-L2412
			name:       "time range 22b",
			path:       tasksPath(userOne),
			fixture:    "Resource/CalDAV/reports/timerangequery/205.xml",
			okHrefs:    []string{object101, object104, object105, object106},
			skipReason: skipQueryExtended,
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L2416-L2429
			name:    "time range 23a",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/timerangequery/203.xml",
			okHrefs: []string{},
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L2436-L2449
			name:       "time range 23b",
			path:       tasksPath(userOne),
			fixture:    "Resource/CalDAV/reports/timerangequery/203.xml",
			skipReason: skipSplitCalendars,
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L2456-L2521
			name:    "time range 24a",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/timerangequery/206.xml",
			okHrefs: []string{
				object5,
				object6,
				object7,
				object8,
				object9,
				object10,
				object11,
				object12,
				object13,
				object14,
				object18,
				object19,
				object20,
				object21,
				object22,
				object23,
				object27,
				object101,
				object102,
				object103,
				object104,
				object105,
				object106,
			},
			skipReason: skipQueryExtended,
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L2529-L2548
			name:       "time range 24b",
			path:       tasksPath(userOne),
			fixture:    "Resource/CalDAV/reports/timerangequery/206.xml",
			okHrefs:    []string{object101, object102, object103, object104, object105, object106},
			skipReason: skipQueryExtended,
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L2555-L2569
			name:         "time range 25",
			path:         calendarPath(userOne),
			fixture:      "Resource/CalDAV/reports/timerangequery/15.xml",
			precondition: "min-date-time",
			skipReason:   "Optional: timerange-low-limit is not planned.",
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L2576-L2590
			name:         "time range 26",
			path:         calendarPath(userOne),
			fixture:      "Resource/CalDAV/reports/timerangequery/16.xml",
			precondition: "min-date-time",
			skipReason:   "Optional: timerange-low-limit is not planned.",
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L2597-L2611
			name:         "time range 27",
			path:         calendarPath(userOne),
			fixture:      "Resource/CalDAV/reports/timerangequery/17.xml",
			precondition: "min-date-time",
			skipReason:   "Optional: timerange-low-limit is not planned.",
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L2618-L2632
			name:         "time range 28",
			path:         calendarPath(userOne),
			fixture:      "Resource/CalDAV/reports/timerangequery/18.xml",
			precondition: "max-date-time",
			skipReason:   "Optional: timerange-high-limit is not planned.",
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L2639-L2653
			name:         "time range 29",
			path:         calendarPath(userOne),
			fixture:      "Resource/CalDAV/reports/timerangequery/19.xml",
			precondition: "max-date-time",
			skipReason:   "Optional: timerange-high-limit is not planned.",
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L2660-L2674
			name:         "time range 30",
			path:         calendarPath(userOne),
			fixture:      "Resource/CalDAV/reports/timerangequery/20.xml",
			precondition: "max-date-time",
			skipReason:   "Optional: timerange-high-limit is not planned.",
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L2681-L2695
			name:         "time range 31",
			path:         calendarPath(userOne),
			fixture:      "Resource/CalDAV/reports/timerangequery/18.xml",
			precondition: "max-date-time",
			skipReason:   "Optional: timerange-high-limit is not planned.",
		},
	}

	runReportCases(t, tests)
}

func TestAppleReportsAlarmTimeRange(t *testing.T) {
	t.Parallel()

	tests := []reportCase{
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L2702-L2716
			name:    "alarm time range 1",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/alarmtimerangequery/1.txt",
			okHrefs: []string{object4},
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L2720-L2734
			name:    "alarm time range 2",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/alarmtimerangequery/2.txt",
			okHrefs: []string{object4},
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L2738-L2753
			name:       "alarm time range 3",
			path:       calendarPath(userOne),
			fixture:    "Resource/CalDAV/reports/alarmtimerangequery/3.txt",
			okHrefs:    []string{object5, object7},
			skipReason: "RFC 4791 9.9: VALARM time-range on recurring events is not implemented.",
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L2757-L2772
			name:       "alarm time range 4",
			path:       calendarPath(userOne),
			fixture:    "Resource/CalDAV/reports/alarmtimerangequery/4.txt",
			okHrefs:    []string{object5, object6},
			skipReason: "RFC 4791 9.9: VALARM time-range on recurring events is not implemented.",
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L2776-L2790
			name:    "alarm time range 5",
			path:    tasksPath(userOne),
			fixture: "Resource/CalDAV/reports/alarmtimerangequery/5.txt",
			okHrefs: []string{object102},
		},
	}

	runReportCases(t, tests)
}

func TestAppleReportsLimitExpand(t *testing.T) {
	t.Parallel()

	tests := []reportCase{
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L2885-L2901
			name:       "limit and expand 1",
			path:       calendarPath(userOne),
			fixture:    "Resource/CalDAV/reports/limitexpand/1.txt",
			okHrefs:    []string{object5, object6, object7},
			skipReason: "Apple compatibility: Apple marks this case as ignored.",
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L2905-L2921
			name:       "limit and expand 2",
			path:       calendarPath(userOne),
			fixture:    "Resource/CalDAV/reports/limitexpand/2.txt",
			okHrefs:    []string{object5, object6, object7},
			skipReason: "Apple compatibility: Apple marks this case as ignored.",
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L2925-L2942
			name:    "limit and expand 3",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/limitexpand/3.txt",
			okHrefs: []string{object5, object6, object7, object19},
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L2946-L2963
			name:    "limit and expand 4",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/limitexpand/4.txt",
			okHrefs: []string{object5, object6, object7, object19},
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L2967-L2993
			name:    "limit and expand 5",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/limitexpand/5.txt",
			okHrefs: []string{object15},
			hrefSets: []reportHrefSet{
				{model: "non-split", ok: []string{object15}, bad: nil},
				{model: "split", ok: nil, bad: nil},
			},
			skipReason: "RFC 4791 9.7.1: comp-filter VFREEBUSY must match only VFREEBUSY components.",
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L2997-L3007
			name:    "limit and expand 6",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/limitexpand/6.xml",
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L3011-L3033
			name:       "limit and expand 7",
			path:       calendarPath(userOne),
			fixture:    "Resource/CalDAV/reports/limitexpand/12.xml",
			okHrefs:    []string{object27},
			contains:   []string{componentVEvent, "DTSTART;VALUE=DATE:$now.year.2:1015"},
			skipReason: "RFC 4791 9.6.5: calendar-data expand is not implemented.",
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L3037-L3059
			name:       "limit and expand 8",
			path:       calendarPath(userOne),
			fixture:    "Resource/CalDAV/reports/limitexpand/13.xml",
			okHrefs:    []string{object18},
			contains:   []string{componentVEvent, "DTSTART;VALUE=DATE:$now.year.2:1017"},
			skipReason: "RFC 4791 9.6.5: calendar-data expand is not implemented.",
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L3063-L3085
			name:       "limit and expand 9a",
			path:       calendarPath(userOne),
			fixture:    "Resource/CalDAV/reports/limitexpand/7.xml",
			okHrefs:    []string{object18},
			contains:   []string{componentVEvent, "DTSTART;VALUE=DATE:$now.year.2:1017"},
			skipReason: "RFC 4791 9.6.5: calendar-data expand is not implemented.",
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L3092-L3114
			name:       "limit and expand 9b",
			path:       calendarPath(userOne),
			fixture:    "Resource/CalDAV/reports/limitexpand/14.xml",
			okHrefs:    []string{object18},
			contains:   []string{componentVEvent, "DTSTART;VALUE=DATE:$now.year.2:1017"},
			skipReason: "Optional: timezones-by-reference is not planned.",
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L3118-L3142
			name:    "limit and expand 10",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/limitexpand/10.xml",
			okHrefs: []string{object18},
			contains: []string{
				componentVEvent,
				"DTSTART;VALUE=DATE:$now.year.2:1017",
				"DTSTART;VALUE=DATE:$now.year.2:1018",
				"RECURRENCE-ID;VALUE=DATE:$now.year.2:1018",
			},
			skipReason: "RFC 4791 9.6.5: calendar-data expand is not implemented.",
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L3146-L3177
			name:    "limit and expand 11",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/limitexpand/8.xml",
			okHrefs: []string{object6},
			contains: []string{
				"DTSTART:$now.year.1:0102T190000Z",
				"DURATION:PT1H",
				"RECURRENCE-ID:$now.year.1:0103T190000Z",
				"DTSTART:$now.year.1:0103T190000Z",
				"RECURRENCE-ID:$now.year.1:0104T190000Z",
				"DTSTART:$now.year.1:0104T210000Z",
			},
			notContains: []string{"VTIMEZONE", "DTSTART:$now.year.1:0104T190000Z"},
			skipReason:  "RFC 4791 9.6.5: calendar-data expand is not implemented.",
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L3203-L3235
			name:    "limit and expand 12",
			path:    calendarPath(userOne),
			fixture: "Resource/CalDAV/reports/limitexpand/9.xml",
			okHrefs: []string{object19, object24, "25.ics"},
			contains: []string{
				"RECURRENCE-ID:$now.year.3:0102T170000Z",
				"RECURRENCE-ID:$now.year.3:0103T170000Z",
				"RECURRENCE-ID:$now.year.3:0104T170000Z",
				"RECURRENCE-ID:$now.year.3:0102T190000Z",
			},
			notContains: []string{
				"DTSTART:$now.year.3:0101T170000Z",
				"RECURRENCE-ID:$now.year.3:0101T170000Z",
				"RECURRENCE-ID:$now.year.3:0101T190000Z",
			},
			skipReason: "RFC 4791 9.6.5: calendar-data expand is not implemented.",
		},
		{
			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/reports.xml#L3250-L3268
			name:       "limit and expand 13",
			path:       calendarPath(userOne),
			fixture:    "Resource/CalDAV/reports/limitexpand/11.xml",
			okHrefs:    []string{"26.ics"},
			skipReason: "RFC 4791 9.6.5: calendar-data expand is not implemented.",
		},
	}

	runReportCases(t, tests)
}
