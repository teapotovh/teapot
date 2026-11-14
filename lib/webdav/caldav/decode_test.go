package caldav

import (
	"encoding/xml"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/teapotovh/teapot/lib/webdav/internal"
)

func unmarshalReport(t *testing.T, body string) *reportReq {
	t.Helper()

	var req reportReq
	require.NoError(t, xml.Unmarshal([]byte(body), &req))

	return &req
}

func decodeQueryFilter(t *testing.T, req *reportReq) CompFilter {
	t.Helper()

	require.NotNil(t, req.Query)

	cf, err := decodeCompFilter(&req.Query.Filter.CompFilter)
	require.NoError(t, err)

	return *cf
}

func decodeReportCalendarData(
	t *testing.T,
	prop *internal.Prop,
) (*calendarDataReq, *CalendarCompRequest) {
	t.Helper()

	require.NotNil(t, prop)

	var data calendarDataReq
	require.NoError(t, prop.Decode(&data))

	decoded, err := decodeCalendarDataReq(&data)
	require.NoError(t, err)

	return &data, decoded
}

func TestReportReqUnmarshalXML(t *testing.T) {
	t.Parallel()

	t.Run("calendar-query", func(t *testing.T) {
		t.Parallel()

		req := unmarshalReport(t, rfc4791CalendarQuery786)
		assert.NotNil(t, req.Query)
		assert.Nil(t, req.Multiget)
	})

	t.Run("calendar-multiget", func(t *testing.T) {
		t.Parallel()

		req := unmarshalReport(t, rfc4791CalendarMultiget791)
		assert.Nil(t, req.Query)
		assert.NotNil(t, req.Multiget)
	})

	t.Run("unsupported caldav root", func(t *testing.T) {
		t.Parallel()

		var req reportReq

		err := xml.Unmarshal(
			[]byte(`<C:free-busy-query xmlns:C="urn:ietf:params:xml:ns:caldav"/>`),
			&req,
		)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrUnsupportedREPORTRoot)
	})

	t.Run("unsupported foreign root", func(t *testing.T) {
		t.Parallel()

		var req reportReq

		err := xml.Unmarshal([]byte(`<D:propfind xmlns:D="DAV:"/>`), &req)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrUnsupportedREPORTRoot)
	})
}

func TestDecodeCalendarQueryTimeRange(t *testing.T) {
	t.Parallel()

	req := unmarshalReport(t, rfc4791CalendarQuery781)
	cf := decodeQueryFilter(t, req)

	assert.Equal(t, testCalendarComponent, cf.Name)
	require.Len(t, cf.Comps, 1)

	vevent := cf.Comps[0]
	assert.Equal(t, testEventComponent, vevent.Name)
	assert.True(t, mustICSTime(t, "20060104T000000Z").Equal(vevent.Start))
	assert.True(t, mustICSTime(t, "20060105T000000Z").Equal(vevent.End))
	assert.Empty(t, vevent.Props)
	assert.Empty(t, vevent.Comps)

	data, compReq := decodeReportCalendarData(t, req.Query.Prop)

	require.NotNil(t, data.Comp)
	assert.Equal(t, testCalendarComponent, data.Comp.Name)
	require.Len(t, data.Comp.Comp, 2)
	assert.Equal(t, testEventComponent, data.Comp.Comp[0].Name)
	assert.Equal(t, "VTIMEZONE", data.Comp.Comp[1].Name)

	assert.False(t, compReq.AllProps)
	assert.False(t, compReq.AllComps)
	assert.Equal(t, []string{"VERSION"}, compReq.Props)
	require.Len(t, compReq.Comps, 2)
	assert.Equal(t, testEventComponent, compReq.Comps[0].Name)
	assert.Equal(t, []string{
		"SUMMARY",
		"UID",
		"DTSTART",
		"DTEND",
		"DURATION",
		"RRULE",
		"RDATE",
		"EXRULE",
		"EXDATE",
		"RECURRENCE-ID",
	}, compReq.Comps[0].Props)
	assert.Equal(t, "VTIMEZONE", compReq.Comps[1].Name)
	assert.Empty(t, compReq.Comps[1].Props)
}

func TestDecodeCalendarDataLimitRecurrenceSet(t *testing.T) {
	t.Parallel()

	req := unmarshalReport(t, rfc4791CalendarQuery782)
	cf := decodeQueryFilter(t, req)

	require.Len(t, cf.Comps, 1)
	assert.Equal(t, testEventComponent, cf.Comps[0].Name)
	assert.True(t, mustICSTime(t, "20060103T000000Z").Equal(cf.Comps[0].Start))
	assert.True(t, mustICSTime(t, "20060105T000000Z").Equal(cf.Comps[0].End))

	data, compReq := decodeReportCalendarData(t, req.Query.Prop)
	assert.Nil(t, data.Comp)
	assert.Nil(t, data.Expand)
	assert.True(t, compReq.AllProps)
	assert.True(t, compReq.AllComps)

	t.Run("limit-recurrence-set", func(t *testing.T) {
		t.Parallel()

		// RFC 4791, section 9.6.2 defines limit-recurrence-set, but
		// calendarDataReq does not decode it.
		t.Skip("TODO: RFC 4791 section 9.6.2 limit-recurrence-set is not implemented")
	})
}

func TestDecodeCalendarDataExpand(t *testing.T) {
	t.Parallel()

	req := unmarshalReport(t, rfc4791CalendarQuery783)
	cf := decodeQueryFilter(t, req)

	require.Len(t, cf.Comps, 1)
	assert.Equal(t, testEventComponent, cf.Comps[0].Name)
	assert.True(t, mustICSTime(t, "20060103T000000Z").Equal(cf.Comps[0].Start))
	assert.True(t, mustICSTime(t, "20060105T000000Z").Equal(cf.Comps[0].End))

	data, compReq := decodeReportCalendarData(t, req.Query.Prop)

	require.NotNil(t, data.Expand)
	assert.True(t, mustICSTime(t, "20060103T000000Z").Equal(time.Time(data.Expand.Start)))
	assert.True(t, mustICSTime(t, "20060105T000000Z").Equal(time.Time(data.Expand.End)))

	require.NotNil(t, compReq.Expand)
	assert.True(t, mustICSTime(t, "20060103T000000Z").Equal(compReq.Expand.Start))
	assert.True(t, mustICSTime(t, "20060105T000000Z").Equal(compReq.Expand.End))

	assert.True(t, compReq.AllProps)
	assert.True(t, compReq.AllComps)
}

func TestDecodePropFilterTextMatch(t *testing.T) {
	t.Parallel()

	req := unmarshalReport(t, rfc4791CalendarQuery786)
	cf := decodeQueryFilter(t, req)

	require.Len(t, cf.Comps, 1)
	require.Len(t, cf.Comps[0].Props, 1)

	prop := cf.Comps[0].Props[0]
	assert.Equal(t, "UID", prop.Name)
	assert.False(t, prop.IsNotDefined)
	require.NotNil(t, prop.TextMatch)
	assert.Equal(t, "DC6C50A017428C5216A2F1CD@example.com", prop.TextMatch.Text)
	assert.False(t, prop.TextMatch.NegateCondition)
	assert.Empty(t, prop.ParamFilter)

	raw := req.Query.Filter.CompFilter.CompFilters[0].PropFilters[0].TextMatch
	require.NotNil(t, raw)
	assert.Equal(t, "i;octet", raw.Collation)
}

func TestDecodeParamFilter(t *testing.T) {
	t.Parallel()

	req := unmarshalReport(t, rfc4791CalendarQuery787)
	cf := decodeQueryFilter(t, req)

	require.Len(t, cf.Comps, 1)
	require.Len(t, cf.Comps[0].Props, 1)

	prop := cf.Comps[0].Props[0]
	assert.Equal(t, "ATTENDEE", prop.Name)
	require.NotNil(t, prop.TextMatch)
	assert.Equal(t, "mailto:lisa@example.com", prop.TextMatch.Text)

	require.Len(t, prop.ParamFilter, 1)

	param := prop.ParamFilter[0]
	assert.Equal(t, "PARTSTAT", param.Name)
	assert.False(t, param.IsNotDefined)
	require.NotNil(t, param.TextMatch)
	assert.Equal(t, "NEEDS-ACTION", param.TextMatch.Text)

	raw := req.Query.Filter.CompFilter.CompFilters[0].PropFilters[0]
	require.NotNil(t, raw.TextMatch)
	assert.Equal(t, "i;ascii-casemap", raw.TextMatch.Collation)
	require.Len(t, raw.ParamFilter, 1)
	require.NotNil(t, raw.ParamFilter[0].TextMatch)
	assert.Equal(t, "i;ascii-casemap", raw.ParamFilter[0].TextMatch.Collation)
}

func TestDecodePropFilterNegateCondition(t *testing.T) {
	t.Parallel()

	req := unmarshalReport(t, rfc4791CalendarQuery789)
	cf := decodeQueryFilter(t, req)

	assert.Equal(t, testCalendarComponent, cf.Name)
	require.Len(t, cf.Comps, 1)

	todo := cf.Comps[0]
	assert.Equal(t, testTodoComponent, todo.Name)
	require.Len(t, todo.Props, 2)

	completed := todo.Props[0]
	assert.Equal(t, "COMPLETED", completed.Name)
	assert.True(t, completed.IsNotDefined)
	assert.Nil(t, completed.TextMatch)

	status := todo.Props[1]
	assert.Equal(t, "STATUS", status.Name)
	assert.False(t, status.IsNotDefined)
	require.NotNil(t, status.TextMatch)
	assert.Equal(t, "CANCELLED", status.TextMatch.Text)
	assert.True(t, status.TextMatch.NegateCondition)
}

func TestDecodeCalendarMultigetHrefs(t *testing.T) {
	t.Parallel()

	req := unmarshalReport(t, rfc4791CalendarMultiget791)

	require.NotNil(t, req.Multiget)
	require.Len(t, req.Multiget.Hrefs, 2)
	assert.Equal(t, "/bernard/work/event-single.ics", req.Multiget.Hrefs[0].Path)
	assert.Equal(t, "/bernard/work/mtg1.ics", req.Multiget.Hrefs[1].Path)

	data, compReq := decodeReportCalendarData(t, req.Multiget.Prop)
	assert.Nil(t, data.Comp)
	assert.Nil(t, data.Expand)
	assert.True(t, compReq.AllProps)
	assert.True(t, compReq.AllComps)
}

func TestDecodeFilterErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		decode func() error
		want   error
	}{
		{
			name: "param-filter is-not-defined with text-match",
			decode: func() error {
				_, err := decodeParamFilter(&paramFilter{
					Name:         "PARTSTAT",
					IsNotDefined: &struct{}{},
					TextMatch:    &textMatch{Text: "NEEDS-ACTION"},
				})

				return err
			},
			want: ErrTextMatchIncompatibleWithIfNotDefined,
		},
		{
			name: "prop-filter is-not-defined with text-match",
			decode: func() error {
				_, err := decodePropFilter(&propFilter{
					Name:         "UID",
					IsNotDefined: &struct{}{},
					TextMatch:    &textMatch{Text: "value"},
				})

				return err
			},
			want: ErrMany1IncompatibleWithIfNotDefined,
		},
		{
			name: "prop-filter is-not-defined with time-range",
			decode: func() error {
				_, err := decodePropFilter(&propFilter{
					Name:         "UID",
					IsNotDefined: &struct{}{},
					TimeRange:    &timeRange{},
				})

				return err
			},
			want: ErrMany1IncompatibleWithIfNotDefined,
		},
		{
			name: "prop-filter is-not-defined with param-filter",
			decode: func() error {
				_, err := decodePropFilter(&propFilter{
					Name:         "UID",
					IsNotDefined: &struct{}{},
					ParamFilter:  []paramFilter{{Name: "PARTSTAT"}},
				})

				return err
			},
			want: ErrMany1IncompatibleWithIfNotDefined,
		},
		{
			name: "comp-filter is-not-defined with time-range",
			decode: func() error {
				_, err := decodeCompFilter(&compFilter{
					Name:         testEventComponent,
					IsNotDefined: &struct{}{},
					TimeRange:    &timeRange{},
				})

				return err
			},
			want: ErrMany2IncompatibleWithIfNotDefined,
		},
		{
			name: "comp-filter is-not-defined with prop-filter",
			decode: func() error {
				_, err := decodeCompFilter(&compFilter{
					Name:         testEventComponent,
					IsNotDefined: &struct{}{},
					PropFilters:  []propFilter{{Name: "UID"}},
				})

				return err
			},
			want: ErrMany2IncompatibleWithIfNotDefined,
		},
		{
			name: "comp-filter is-not-defined with comp-filter",
			decode: func() error {
				_, err := decodeCompFilter(&compFilter{
					Name:         testEventComponent,
					IsNotDefined: &struct{}{},
					CompFilters:  []compFilter{{Name: testAlarmComponent}},
				})

				return err
			},
			want: ErrMany2IncompatibleWithIfNotDefined,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.decode()
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.want)
		})
	}
}

func TestDecodePropFilterMultipleTextMatch(t *testing.T) {
	t.Parallel()

	// RFC 4791, section 9.7.2 allows at most one text-match in a
	// prop-filter. propFilter keeps a single textMatch, so duplicate
	// elements are silently overwritten instead of being rejected.
	t.Skip("TODO: RFC 4791 section 9.7.2 duplicate text-match elements are not rejected")
}

func TestNegateConditionUnmarshal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		value   string
		want    negateCondition
		wantErr error
	}{
		{value: "yes", want: true},
		{value: "no", want: false},
		{value: "maybe", wantErr: ErrInvalidNegateCondition},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			t.Parallel()

			var match textMatch

			body := `<text-match xmlns="urn:ietf:params:xml:ns:caldav" negate-condition="` + tt.value + `">text</text-match>`
			err := xml.Unmarshal([]byte(body), &match)

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, match.NegateCondition)
		})
	}
}
