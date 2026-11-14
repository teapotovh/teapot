package caldavtest

import (
	"bytes"
	"encoding/xml"
	"errors"
	"net/http"
	"net/http/httptest"
	"path"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	ics "github.com/arran4/golang-ical"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	errInvalidFreeBusyTime     = errors.New("invalid free-busy time")
	errInvalidFreeBusyDuration = errors.New("invalid free-busy duration")
)

const (
	componentVEvent    = "VEVENT"
	componentVTodo     = "VTODO"
	componentVJournal  = "VJOURNAL"
	componentVFreeBusy = "VFREEBUSY"

	skipSplitCalendars = "Optional: split-calendars is not planned."
	skipQueryExtended  = "Optional: query-extended is not planned."

	skipDepthInfinity = "Apple compatibility: RFC 4918 9.1 permits accepting Depth infinity; " +
		"accept and skip this Apple-specific refusal case."
	skipCollectionListing = "Apple compatibility: RFC 4918 does not define a collection listing " +
		"representation; not planned."
	skipConditionalPropfind = "Apple compatibility: RFC 9110 13.1.3 says to ignore If-Modified-Since " +
		"on PROPFIND."
	skipPermissiveParsing = "Apple compatibility: permissive parsing is not an RFC requirement."
	skipDuplicateAlarms   = "Apple extension: duplicate alarm removal is not planned."
	skipProppatch         = "RFC 4918 9.2: PROPPATCH is not implemented."
	skipFreeBusy          = "RFC 4791 7.10: free-busy-query is required and not implemented."
)

func putAppleFixture(t *testing.T, srv *httptest.Server, user string, objectPath string, fixture string) httpResponse {
	t.Helper()

	return putObject(t, srv, user, objectPath, loadAppleFixture(t, fixture, nil))
}

func requireSuccess(t *testing.T, res httpResponse) {
	t.Helper()

	require.GreaterOrEqual(t, res.Status, 200)
	require.Less(t, res.Status, 300, "body: %s", res.Body)
}

func multiStatusHrefBases(t *testing.T, body string) []string {
	t.Helper()

	ms := parseMultiStatus(t, body)

	var names []string

	for _, response := range ms.Responses {
		for _, href := range response.Hrefs {
			names = append(names, path.Base(strings.TrimSuffix(href, "/")))
		}
	}

	sort.Strings(names)

	return names
}

func assertHrefBases(t *testing.T, res httpResponse, want []string) {
	t.Helper()

	require.Equal(t, http.StatusMultiStatus, res.Status, "body: %s", res.Body)

	got := multiStatusHrefBases(t, res.Body)

	expected := append([]string(nil), want...)
	sort.Strings(expected)

	assert.Equal(t, expected, got, "body: %s", res.Body)
}

func assertReportHrefs(t *testing.T, res httpResponse, okHrefs []string, badHrefs []string) {
	t.Helper()

	require.Equal(t, http.StatusMultiStatus, res.Status, "body: %s", res.Body)

	ms := parseMultiStatus(t, res.Body)

	expected := make(map[string]int, len(okHrefs)+len(badHrefs))
	for _, href := range okHrefs {
		expected[path.Clean(href)] = http.StatusOK
	}

	for _, href := range badHrefs {
		expected[path.Clean(href)] = http.StatusNotFound
	}

	got := make(map[string][]int, len(ms.Responses))
	for _, response := range ms.Responses {
		statuses := make([]int, 0, len(response.PropStats)+1)
		for _, propstat := range response.PropStats {
			statuses = append(statuses, statusCode(t, propstat.Status))
		}

		if len(statuses) == 0 && response.Status != "" {
			statuses = append(statuses, statusCode(t, response.Status))
		}

		for _, href := range response.Hrefs {
			got[path.Clean(href)] = statuses
		}
	}

	expectedHrefs := make([]string, 0, len(expected))
	for href := range expected {
		expectedHrefs = append(expectedHrefs, href)
	}

	gotHrefs := make([]string, 0, len(got))
	for href := range got {
		gotHrefs = append(gotHrefs, href)
	}

	assert.ElementsMatch(t, expectedHrefs, gotHrefs, "body: %s", res.Body)

	for href, want := range expected {
		statuses, ok := got[href]
		require.True(t, ok, "missing response for %q; body: %s", href, res.Body)
		require.Len(t, statuses, 1, "unexpected propstat count for %q; body: %s", href, res.Body)
		assert.Equal(t, want, statuses[0], "unexpected status for %q; body: %s", href, res.Body)
	}
}

func assertOKHrefs(t *testing.T, res httpResponse, hrefs []string) {
	t.Helper()

	assertReportHrefs(t, res, hrefs, nil)
}

func assertFreeBusy(t *testing.T, body string, busy []string, tentative []string, unavailable []string) {
	t.Helper()

	year := strconv.Itoa(time.Now().Year())

	got := parseFreeBusy(body)

	assert.ElementsMatch(t, normalizeFreeBusyPeriods(year, busy), got["BUSY"], "body: %s", body)
	assert.ElementsMatch(t, normalizeFreeBusyPeriods(year, tentative), got["BUSY-TENTATIVE"], "body: %s", body)
	assert.ElementsMatch(
		t,
		normalizeFreeBusyPeriods(year, unavailable),
		got["BUSY-UNAVAILABLE"],
		"body: %s",
		body,
	)
}

func normalizeFreeBusyPeriods(year string, periods []string) []string {
	out := make([]string, 0, len(periods))
	for _, period := range periods {
		parts := strings.SplitN(period, "/", 2)
		if len(parts) != 2 {
			out = append(out, period)
			continue
		}

		out = append(out, prefixFreeBusyYear(year, parts[0])+"/"+prefixFreeBusyYear(year, parts[1]))
	}

	sort.Strings(out)

	return out
}

func prefixFreeBusyYear(year string, value string) string {
	if len(value) >= 15 {
		return value
	}

	return year + value
}

func parseFreeBusy(body string) map[string][]string {
	var unfolded []string

	for line := range strings.SplitSeq(body, "\n") {
		line = strings.TrimRight(line, "\r")

		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			if len(unfolded) > 0 {
				unfolded[len(unfolded)-1] += line[1:]
			}

			continue
		}

		unfolded = append(unfolded, line)
	}

	result := make(map[string][]string)

	for _, line := range unfolded {
		namePart, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}

		name, params := parsePropertyName(namePart)
		if name != "FREEBUSY" {
			continue
		}

		fbType := "BUSY"

		for _, param := range params {
			if key, val, found := strings.Cut(param, "="); found && strings.EqualFold(key, "FBTYPE") {
				fbType = strings.ToUpper(val)
			}
		}

		for period := range strings.SplitSeq(value, ",") {
			start, end, ok := parseFreeBusyPeriod(period)
			if !ok {
				continue
			}

			result[fbType] = append(result[fbType], start+"/"+end)
		}
	}

	for key := range result {
		sort.Strings(result[key])
	}

	return result
}

func parsePropertyName(value string) (string, []string) {
	parts := strings.Split(value, ";")
	name := strings.ToUpper(parts[0])

	return name, parts[1:]
}

func parseFreeBusyPeriod(period string) (string, string, bool) {
	startValue, endValue, ok := strings.Cut(period, "/")
	if !ok {
		return "", "", false
	}

	start, err := parseFreeBusyTime(startValue)
	if err != nil {
		return "", "", false
	}

	var end time.Time

	if strings.HasPrefix(endValue, "P") {
		duration, err := parseFreeBusyDuration(endValue)
		if err != nil {
			return "", "", false
		}

		end = start.Add(duration)
	} else {
		end, err = parseFreeBusyTime(endValue)
		if err != nil {
			return "", "", false
		}
	}

	return start.UTC().Format("20060102T150405Z"), end.UTC().Format("20060102T150405Z"), true
}

func parseFreeBusyTime(value string) (time.Time, error) {
	for _, layout := range []string{"20060102T150405Z", "20060102T150405", "20060102"} {
		if parsed, err := time.ParseInLocation(layout, value, time.UTC); err == nil {
			return parsed, nil
		}
	}

	return time.Time{}, errInvalidFreeBusyTime
}

func parseFreeBusyDuration(value string) (time.Duration, error) {
	if !strings.HasPrefix(value, "P") {
		return 0, errInvalidFreeBusyDuration
	}

	value = strings.TrimPrefix(value, "P")

	var (
		total  time.Duration
		number strings.Builder
	)

	for _, r := range value {
		if r >= '0' && r <= '9' {
			number.WriteRune(r)
			continue
		}

		if r == 'T' {
			continue
		}

		n, err := strconv.Atoi(number.String())
		if err != nil {
			return 0, errInvalidFreeBusyDuration
		}

		number.Reset()

		switch r {
		case 'D':
			total += time.Duration(n) * 24 * time.Hour
		case 'H':
			total += time.Duration(n) * time.Hour
		case 'M':
			total += time.Duration(n) * time.Minute
		case 'S':
			total += time.Duration(n) * time.Second
		default:
			return 0, errInvalidFreeBusyDuration
		}
	}

	return total, nil
}

func assertPropstat(t *testing.T, body string, href string, status int) multiStatusProp {
	t.Helper()

	ms := parseMultiStatus(t, body)

	response, ok := ms.findResponse(href)
	require.True(t, ok, "missing response for %q; body: %s", href, body)

	propstat, ok := response.findPropstat(t, status)
	require.True(t, ok, "missing %d propstat for %q; body: %s", status, href, body)

	return propstat.Prop
}

func assertResourceType(t *testing.T, prop multiStatusProp, calendar bool) {
	t.Helper()

	require.NotNil(t, prop.ResourceType, "missing resourcetype")
	assert.NotNil(t, prop.ResourceType.Collection, "resourcetype must contain collection")

	if calendar {
		assert.NotNil(t, prop.ResourceType.Calendar, "resourcetype must contain calendar")
	}
}

func assertSupportedComponents(t *testing.T, prop multiStatusProp, want []string) {
	t.Helper()

	require.NotNil(t, prop.SupportedComponents, "missing supported-calendar-component-set")

	got := make([]string, 0, len(prop.SupportedComponents.Comp))
	for _, comp := range prop.SupportedComponents.Comp {
		got = append(got, comp.Name)
	}

	assert.ElementsMatch(t, want, got)
}

func serializeCalendar(t *testing.T, data string) string {
	t.Helper()

	cal, err := ics.ParseCalendar(strings.NewReader(data))
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, cal.SerializeTo(&buf))

	return buf.String()
}

func assertCalendarDataEqual(t *testing.T, got string, want string) {
	t.Helper()

	assert.Equal(t, serializeCalendar(t, want), serializeCalendar(t, got))
}

type genericProp struct {
	XMLName xml.Name
	Value   string `xml:",chardata"`
}

type genericPropValues struct {
	Props []genericProp `xml:",any"`
}

type genericMultiStatus struct {
	Responses []struct {
		Hrefs     []string `xml:"DAV: href"`
		PropStats []struct {
			Status string            `xml:"DAV: status"`
			Prop   genericPropValues `xml:"DAV: prop"`
		} `xml:"DAV: propstat"`
	} `xml:"DAV: response"`
}

func statusCode(t *testing.T, status string) int {
	t.Helper()

	fields := strings.Fields(status)
	require.GreaterOrEqual(t, len(fields), 2)

	code, err := strconv.Atoi(fields[1])
	require.NoError(t, err)

	return code
}

func assertPropNames(t *testing.T, body string, href string, status int, names ...xml.Name) {
	t.Helper()

	var ms genericMultiStatus
	require.NoError(t, xml.Unmarshal([]byte(body), &ms))

	for _, response := range ms.Responses {
		if !slicesContain(response.Hrefs, href) {
			continue
		}

		for _, propstat := range response.PropStats {
			if statusCode(t, propstat.Status) != status {
				continue
			}

			got := make(map[xml.Name]bool, len(propstat.Prop.Props))
			for _, prop := range propstat.Prop.Props {
				got[prop.XMLName] = true
			}

			for _, name := range names {
				assert.True(t, got[name], "missing property %s in %d propstat", name.Local, status)
			}

			return
		}
	}

	require.FailNowf(t, "missing propstat", "no %d propstat for %q", status, href)
}

func assertPropValue(t *testing.T, body string, href string, status int, name xml.Name, want string) {
	t.Helper()

	var ms genericMultiStatus
	require.NoError(t, xml.Unmarshal([]byte(body), &ms))

	for _, response := range ms.Responses {
		if !slicesContain(response.Hrefs, href) {
			continue
		}

		for _, propstat := range response.PropStats {
			if statusCode(t, propstat.Status) != status {
				continue
			}

			for _, prop := range propstat.Prop.Props {
				if prop.XMLName != name {
					continue
				}

				assert.Equal(t, want, strings.TrimSpace(prop.Value))

				return
			}
		}
	}

	require.FailNowf(t, "missing property", "no %s in %d propstat for %q", name.Local, status, href)
}

func assertPropAbsent(t *testing.T, body string, href string, name xml.Name) {
	t.Helper()

	var ms genericMultiStatus
	require.NoError(t, xml.Unmarshal([]byte(body), &ms))

	for _, response := range ms.Responses {
		if !slicesContain(response.Hrefs, href) {
			continue
		}

		for _, propstat := range response.PropStats {
			if statusCode(t, propstat.Status) != http.StatusOK {
				continue
			}

			for _, prop := range propstat.Prop.Props {
				assert.NotEqual(t, name, prop.XMLName, "unexpected property %s", name.Local)
			}
		}
	}
}

func assertAllResponsesHaveProps(t *testing.T, body string, status int, names ...xml.Name) {
	t.Helper()

	var ms genericMultiStatus
	require.NoError(t, xml.Unmarshal([]byte(body), &ms))

	for _, response := range ms.Responses {
		for _, propstat := range response.PropStats {
			if statusCode(t, propstat.Status) != status {
				continue
			}

			got := make(map[xml.Name]bool, len(propstat.Prop.Props))
			for _, prop := range propstat.Prop.Props {
				got[prop.XMLName] = true
			}

			for _, name := range names {
				assert.True(t, got[name], "response %v missing property %s", response.Hrefs, name.Local)
			}
		}
	}
}

func slicesContain(haystack []string, needle string) bool {
	for _, value := range haystack {
		if sameHref(value, needle) {
			return true
		}
	}

	return false
}

func sameHref(a string, b string) bool {
	return path.Clean(a) == path.Clean(b)
}

const (
	object1   = "1.ics"
	object2   = "2.ics"
	object3   = "3.ics"
	object4   = "4.ics"
	object5   = "5.ics"
	object6   = "6.ics"
	object7   = "7.ics"
	object8   = "8.ics"
	object9   = "9.ics"
	object10  = "10.ics"
	object11  = "11.ics"
	object12  = "12.ics"
	object13  = "13.ics"
	object14  = "14.ics"
	object15  = "15.ics"
	object18  = "18.ics"
	object19  = "19.ics"
	object20  = "20.ics"
	object21  = "21.ics"
	object22  = "22.ics"
	object23  = "23.ics"
	object24  = "24.ics"
	object27  = "27.ics"
	object101 = "101.ics"
	object102 = "102.ics"
	object103 = "103.ics"
	object104 = "104.ics"
	object105 = "105.ics"
	object106 = "106.ics"
)

const (
	fixturePropfinds1     = "Resource/CalDAV/propfinds/1.txt"
	fixtureDelete1        = "Resource/CalDAV/delete/1.txt"
	fixtureFloating1      = "Resource/CalDAV/floating/1.txt"
	fixtureRecurrenceput1 = "Resource/CalDAV/recurrenceput/1.txt"
	fixtureMkcalendar1    = "Resource/CalDAV/mkcalendar/1.txt"
	fixtureDisplayName    = "Resource/Common/PROPFIND/displayname.xml"
	fixtureFloating2      = "Resource/CalDAV/floating/2.txt"
	fixturePropfinds2     = "Resource/CalDAV/propfinds/2.txt"
	fixturePropfinds3     = "Resource/CalDAV/propfinds/3.txt"
	fixtureRecurrenceput9 = "Resource/CalDAV/recurrenceput/9.ics"
)
