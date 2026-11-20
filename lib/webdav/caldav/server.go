package caldav

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	ics "github.com/arran4/golang-ical"

	"github.com/teapotovh/teapot/lib/webdav"
	daverr "github.com/teapotovh/teapot/lib/webdav/error"
	"github.com/teapotovh/teapot/lib/webdav/internal"
)

const MIMEType = "text/calendar"

var (
	ErrInvalidCalendarPath                   = errors.New("invalid calendar path (must be under calendar home-set)")
	ErrTextMatchIncompatibleWithIfNotDefined = errors.New("if is-not-defined is provided, text-match can't be provided")
	ErrMultipleTextMatch                     = errors.New("prop-filter can have at most one text-match")
	ErrMany1IncompatibleWithIfNotDefined     = errors.New(
		"if is-not-defined is provided, text-match, time-range, or param-filter can't be provided",
	)
	ErrMany2IncompatibleWithIfNotDefined = errors.New(
		"if is-not-defined is provided, time-range, prop-filter, or comp-filter can't be provided",
	)

	// ErrInvalidTimeZone reports a malformed CALDAV:timezone element.
	ErrInvalidTimeZone = errors.New("invalid timezone element")
)

// Backend is a CalDAV server backend.
type Backend interface {
	CalendarHomeSetPath(ctx context.Context) (string, error)

	PutCalendar(ctx context.Context, calendar *Calendar) (created bool, err error)
	ListCalendars(ctx context.Context) ([]Calendar, error)
	GetCalendar(ctx context.Context, path string) (*Calendar, error)

	GetCalendarObject(
		ctx context.Context,
		path string,
		req *CalendarCompRequest,
		pre webdav.Preconditions,
	) (*CalendarObject, webdav.Outcome, error)
	ListCalendarObjects(ctx context.Context, path string, req *CalendarCompRequest) ([]CalendarObject, error)
	QueryCalendarObjects(ctx context.Context, path string, query *CalendarQuery) ([]CalendarObject, error)
	PutCalendarObject(
		ctx context.Context,
		path string,
		calendar *ics.Calendar,
		pre webdav.Preconditions,
	) (*CalendarObject, bool, error)
	DeleteCalendarObject(ctx context.Context, path string, pre webdav.Preconditions) error

	webdav.UserPrincipalBackend
}

// Handler handles CalDAV HTTP requests. It can be used to create a CalDAV
// server.
type Handler struct {
	Backend Backend
	Prefix  string
}

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.Backend == nil {
		http.Error(w, "caldav: no backend available", http.StatusInternalServerError)
		return
	}

	var err error

	switch r.Method {
	case webdav.MethodReport:
		err = h.handleReport(w, r)
	default:
		b := backend{
			Backend: h.Backend,
			Prefix:  strings.TrimSuffix(h.Prefix, "/"),
		}
		hh := internal.Handler{Backend: &b}
		hh.ServeHTTP(w, r)
	}

	if err != nil {
		internal.ServeError(w, err)
	}
}

func (h *Handler) handleReport(w http.ResponseWriter, r *http.Request) error {
	var report reportReq
	if err := internal.DecodeXMLRequest(r, &report); err != nil {
		if errors.Is(err, ErrUnsupportedREPORTRoot) {
			return newDAVError(http.StatusForbidden, internal.SupportedReportName)
		}

		return err
	}

	if report.Query != nil {
		return h.handleQuery(r, w, report.Query)
	} else if report.Multiget != nil {
		return h.handleMultiget(r.Context(), w, report.Multiget)
	} else if report.FreeBusy != nil {
		return h.handleFreeBusy(r, w, report.FreeBusy)
	}

	return daverr.HTTPErrorf(
		http.StatusBadRequest,
		"caldav: expected calendar-query, calendar-multiget or free-busy-query element in REPORT request",
	)
}

func decodeTextMatch(el *textMatch) *TextMatch {
	return &TextMatch{
		Text:            el.Text,
		NegateCondition: bool(el.NegateCondition),
	}
}

func decodeParamFilter(el *paramFilter) (*ParamFilter, error) {
	pf := &ParamFilter{Name: el.Name}
	if el.IsNotDefined != nil {
		if el.TextMatch != nil {
			return nil, fmt.Errorf("caldav: failed to parse param-filter: %w", ErrTextMatchIncompatibleWithIfNotDefined)
		}

		pf.IsNotDefined = true
	}

	if el.TextMatch != nil {
		pf.TextMatch = decodeTextMatch(el.TextMatch)
	}

	return pf, nil
}

func decodePropFilter(el *propFilter) (*PropFilter, error) {
	if len(el.TextMatch) > 1 {
		return nil, daverr.HTTPErrorf(
			http.StatusBadRequest,
			"caldav: failed to parse prop-filter: %w",
			ErrMultipleTextMatch,
		)
	}

	pf := &PropFilter{Name: el.Name}
	if el.IsNotDefined != nil {
		if len(el.TextMatch) > 0 || el.TimeRange != nil || len(el.ParamFilter) > 0 {
			return nil, fmt.Errorf("caldav: failed to parse prop-filter: %w", ErrMany1IncompatibleWithIfNotDefined)
		}

		pf.IsNotDefined = true
	}

	if len(el.TextMatch) > 0 {
		pf.TextMatch = decodeTextMatch(&el.TextMatch[0])
	}

	if el.TimeRange != nil {
		pf.Start = time.Time(el.TimeRange.Start)
		pf.End = time.Time(el.TimeRange.End)
	}

	for _, paramEl := range el.ParamFilter {
		paramFi, err := decodeParamFilter(&paramEl)
		if err != nil {
			return nil, err
		}

		pf.ParamFilter = append(pf.ParamFilter, *paramFi)
	}

	return pf, nil
}

func decodeCompFilter(el *compFilter) (*CompFilter, error) {
	cf := &CompFilter{Name: el.Name}
	if el.IsNotDefined != nil {
		if el.TimeRange != nil || len(el.PropFilters) > 0 || len(el.CompFilters) > 0 {
			return nil, fmt.Errorf("caldav: failed to parse comp-filter: %w", ErrMany2IncompatibleWithIfNotDefined)
		}

		cf.IsNotDefined = true
	}

	if el.TimeRange != nil {
		cf.Start = time.Time(el.TimeRange.Start)
		cf.End = time.Time(el.TimeRange.End)
	}

	for _, pfEl := range el.PropFilters {
		pf, err := decodePropFilter(&pfEl)
		if err != nil {
			return nil, err
		}

		cf.Props = append(cf.Props, *pf)
	}

	for _, childEl := range el.CompFilters {
		child, err := decodeCompFilter(&childEl)
		if err != nil {
			return nil, err
		}

		cf.Comps = append(cf.Comps, *child)
	}

	return cf, nil
}

// decodeTimeZone parses the CALDAV:timezone element, as defined in RFC 4791,
// section 9.8. It returns nil when the element is absent.
func decodeTimeZone(el *calendarTimeZone) (*time.Location, error) {
	if el == nil {
		return nil, nil //nolint:nilnil // A nil location means the element is absent.
	}

	return parseTimeZone(el.Data)
}

// parseTimeZone parses a VTIMEZONE iCalendar value. It returns UTC when the
// value is empty.
func parseTimeZone(data string) (*time.Location, error) {
	if strings.TrimSpace(data) == "" {
		return time.UTC, nil
	}

	cal, err := ics.ParseCalendar(strings.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("caldav: failed to parse timezone: %w: %w", ErrInvalidTimeZone, err)
	}

	timezones := cal.Timezones()
	if len(timezones) == 0 {
		return nil, fmt.Errorf("caldav: failed to parse timezone: %w: no VTIMEZONE", ErrInvalidTimeZone)
	}

	prop := timezones[0].GetProperty(ics.ComponentPropertyTzid)
	if prop == nil {
		return nil, fmt.Errorf("caldav: failed to parse timezone: %w: no TZID", ErrInvalidTimeZone)
	}

	location, err := time.LoadLocation(unquoteValue(prop.Value))
	if err != nil {
		return nil, fmt.Errorf("caldav: failed to parse timezone: %w: %w", ErrInvalidTimeZone, err)
	}

	return location, nil
}

func decodeComp(comp *comp) (*CalendarCompRequest, error) {
	if comp == nil {
		return nil, daverr.HTTPErrorf(http.StatusBadRequest, "caldav: unexpected empty calendar-data in request")
	}

	if comp.Allprop != nil && len(comp.Prop) > 0 {
		return nil, daverr.HTTPErrorf(
			http.StatusBadRequest,
			"caldav: only one of allprop or prop can be specified in calendar-data",
		)
	}

	if comp.Allcomp != nil && len(comp.Comp) > 0 {
		return nil, daverr.HTTPErrorf(
			http.StatusBadRequest,
			"caldav: only one of allcomp or comp can be specified in calendar-data",
		)
	}

	req := &CalendarCompRequest{
		Name:     comp.Name,
		AllProps: comp.Allprop != nil,
		AllComps: comp.Allcomp != nil,
	}
	for _, p := range comp.Prop {
		req.Props = append(req.Props, p.Name)
	}

	for _, c := range comp.Comp {
		comp, err := decodeComp(&c)
		if err != nil {
			return nil, err
		}

		req.Comps = append(req.Comps, *comp)
	}

	return req, nil
}

func decodeCalendarDataReq(calendarData *calendarDataReq) (*CalendarCompRequest, error) {
	req := &CalendarCompRequest{
		AllProps: true,
		AllComps: true,
	}

	if calendarData.Comp != nil {
		decoded, err := decodeComp(calendarData.Comp)
		if err != nil {
			return nil, err
		}

		req = decoded
	}

	if calendarData.Expand != nil {
		req.Expand = &CalendarExpandRequest{
			Start: time.Time(calendarData.Expand.Start),
			End:   time.Time(calendarData.Expand.End),
		}
	}

	return req, nil
}

// componentNames extracts the component names from a supported-calendar-component-set
// element, ignoring empty names.
func componentNames(components []comp) []string {
	names := make([]string, 0, len(components))

	for _, component := range components {
		if component.Name != "" {
			names = append(names, component.Name)
		}
	}

	return names
}

func (h *Handler) handleQuery(r *http.Request, w http.ResponseWriter, query *calendarQuery) error {
	var q CalendarQuery
	// TODO: calendar-data in query.Prop
	cf, err := decodeCompFilter(&query.Filter.CompFilter)
	if err != nil {
		return err
	}

	q.CompFilter = *cf

	q.TimeZone, err = decodeTimeZone(query.TimeZone)
	if err != nil {
		return err
	}

	if q.TimeZone == nil {
		q.TimeZone, err = h.defaultTimeZone(r.Context(), r.URL.Path)
		if err != nil {
			return err
		}
	}

	cos, err := h.Backend.QueryCalendarObjects(r.Context(), r.URL.Path, &q)
	if err != nil {
		return err
	}

	var resps []internal.Response

	for _, co := range cos {
		b := backend{
			Backend: h.Backend,
			Prefix:  strings.TrimSuffix(h.Prefix, "/"),
		}
		propfind := internal.PropFind{
			Prop:     query.Prop,
			AllProp:  query.AllProp,
			PropName: query.PropName,
		}

		resp, err := b.propFindCalendarObject(r.Context(), &propfind, &co)
		if err != nil {
			return err
		}

		resps = append(resps, *resp)
	}

	ms := internal.NewMultiStatus(resps...)

	return internal.ServeMultiStatus(w, ms)
}

func (h *Handler) handleMultiget(ctx context.Context, w http.ResponseWriter, multiget *calendarMultiget) error {
	var dataReq CalendarCompRequest

	if multiget.Prop != nil {
		var calendarData calendarDataReq
		if err := multiget.Prop.Decode(&calendarData); err != nil && !daverr.IsNotFound(err) {
			return err
		}

		decoded, err := decodeCalendarDataReq(&calendarData)
		if err != nil {
			return err
		}

		dataReq = *decoded
	}

	var resps []internal.Response

	for _, href := range multiget.Hrefs {
		co, _, err := h.Backend.GetCalendarObject(ctx, href.Path, &dataReq, webdav.Preconditions{})
		if err != nil {
			resp := internal.NewErrorResponse(href.Path, err)
			resps = append(resps, *resp)

			continue
		}

		b := backend{
			Backend: h.Backend,
			Prefix:  strings.TrimSuffix(h.Prefix, "/"),
		}
		propfind := internal.PropFind{
			Prop:     multiget.Prop,
			AllProp:  multiget.AllProp,
			PropName: multiget.PropName,
		}

		resp, err := b.propFindCalendarObject(ctx, &propfind, co)
		if err != nil {
			return err
		}

		resps = append(resps, *resp)
	}

	ms := internal.NewMultiStatus(resps...)

	return internal.ServeMultiStatus(w, ms)
}

func (h *Handler) handleFreeBusy(r *http.Request, w http.ResponseWriter, query *freeBusyQuery) error {
	start := time.Time(query.TimeRange.Start)
	end := time.Time(query.TimeRange.End)

	if start.IsZero() || end.IsZero() {
		return daverr.HTTPErrorf(
			http.StatusBadRequest,
			"caldav: free-busy-query requires a time-range with start and end",
		)
	}

	if !end.After(start) {
		return daverr.HTTPErrorf(
			http.StatusBadRequest,
			"caldav: free-busy-query time-range end must be after start",
		)
	}

	cal, err := h.Backend.GetCalendar(r.Context(), r.URL.Path)
	if err != nil {
		return err
	}

	location, err := calendarLocation(cal)
	if err != nil {
		return err
	}

	cos, err := h.Backend.ListCalendarObjects(r.Context(), r.URL.Path, &CalendarCompRequest{})
	if err != nil {
		return err
	}

	periods, err := computeFreeBusy(cos, start, end, location)
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", freeBusyContentType)

	return writeFreeBusy(w, start, end, periods)
}

// calendarLocation returns the time zone of the calendar collection. It returns
// UTC when the CALDAV:calendar-timezone property is absent.
func calendarLocation(cal *Calendar) (*time.Location, error) {
	if cal == nil {
		return time.UTC, nil
	}

	value, ok := cal.DeadProperties[calendarTimeZoneName]
	if !ok {
		return time.UTC, nil
	}

	raw, err := unmarshalProperty(value)
	if err != nil {
		return nil, err
	}

	var prop calendarTimeZoneProp
	if err := raw.Decode(&prop); err != nil {
		return nil, fmt.Errorf("caldav: failed to decode calendar-timezone: %w", err)
	}

	return parseTimeZone(prop.Data)
}

// defaultTimeZone returns the time zone of the calendar collection. It returns
// UTC when the collection does not exist or has no calendar-timezone property.
func (h *Handler) defaultTimeZone(ctx context.Context, calendarPath string) (*time.Location, error) {
	cal, err := h.Backend.GetCalendar(ctx, calendarPath)
	if err != nil {
		if daverr.IsNotFound(err) {
			return time.UTC, nil
		}

		return nil, err
	}

	return calendarLocation(cal)
}

type backend struct {
	Backend Backend
	Prefix  string
}

type resourceType int

const (
	resourceTypeRoot resourceType = iota
	resourceTypeUserPrincipal
	resourceTypeCalendarHomeSet
	resourceTypeCalendar
	resourceTypeCalendarObject
)

func (b *backend) Options(r *http.Request) (caps []string, allow []string, err error) {
	caps = []string{"calendar-access"}

	switch b.resourceTypeAtPath(r.URL.Path) {
	case resourceTypeRoot, resourceTypeUserPrincipal:
		return caps, []string{http.MethodOptions, webdav.MethodPropFind}, nil
	case resourceTypeCalendarHomeSet:
		return caps, []string{http.MethodOptions, webdav.MethodPropFind, webdav.MethodMkcalendar}, nil
	case resourceTypeCalendar:
		return caps, []string{http.MethodOptions, webdav.MethodPropFind, webdav.MethodReport}, nil
	case resourceTypeCalendarObject:
		var dataReq CalendarCompRequest

		_, _, err = b.Backend.GetCalendarObject(r.Context(), r.URL.Path, &dataReq, webdav.Preconditions{})

		httpErr := &daverr.HTTPError{}
		if errors.As(err, &httpErr) {
			return caps, []string{http.MethodOptions, http.MethodPut}, nil
		} else if err != nil {
			return nil, nil, err
		}

		return caps, []string{
			http.MethodOptions,
			http.MethodHead,
			http.MethodGet,
			http.MethodPut,
			http.MethodDelete,
			webdav.MethodPropFind,
		}, nil
	}

	return caps, []string{http.MethodOptions, webdav.MethodPropFind}, nil
}

func (b *backend) HeadGet(w http.ResponseWriter, r *http.Request) error {
	var dataReq CalendarCompRequest
	if r.Method != http.MethodHead {
		dataReq.AllProps = true
	}

	pre, err := webdav.PreconditionsFromHeader(r.Header)
	if err != nil {
		return err
	}

	co, outcome, err := b.Backend.GetCalendarObject(r.Context(), r.URL.Path, &dataReq, pre)
	if err != nil {
		return err
	}

	if co.ETag != "" {
		w.Header().Set("ETag", internal.ETag(co.ETag).String())
	}

	if !co.ModTime.IsZero() {
		w.Header().Set("Last-Modified", co.ModTime.UTC().Format(http.TimeFormat))
	}

	if outcome == webdav.NotModified {
		w.WriteHeader(http.StatusNotModified)
		return nil
	}

	if outcome == webdav.PreconditionFailed {
		return daverr.HTTPErrorf(http.StatusPreconditionFailed, "caldav: precondition failed")
	}

	w.Header().Set("Content-Type", MIMEType)

	if co.ContentLength > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(co.ContentLength, 10))
	}

	if r.Method != http.MethodHead {
		return co.Data.SerializeTo(w)
	}

	return nil
}

//nolint:gocyclo
func (b *backend) PropFind(
	r *http.Request,
	propfind *internal.PropFind,
	depth internal.Depth,
) (*internal.MultiStatus, error) {
	resType := b.resourceTypeAtPath(r.URL.Path)

	var (
		dataReq CalendarCompRequest
		resps   []internal.Response
	)

	switch resType {
	case resourceTypeRoot:
		resp, err := b.propFindRoot(r.Context(), propfind)
		if err != nil {
			return nil, err
		}

		resps = append(resps, *resp)
	case resourceTypeUserPrincipal:
		principalPath, err := b.Backend.CurrentUserPrincipal(r.Context())
		if err != nil {
			return nil, err
		}

		if samePath(r.URL.Path, principalPath) {
			resp, err := b.propFindUserPrincipal(r.Context(), propfind)
			if err != nil {
				return nil, err
			}

			resps = append(resps, *resp)
			if depth != internal.DepthZero {
				resp, err := b.propFindHomeSet(r.Context(), propfind)
				if err != nil {
					return nil, err
				}

				resps = append(resps, *resp)

				if depth == internal.DepthInfinity {
					resps_, err := b.propFindAllCalendars(r.Context(), propfind, true)
					if err != nil {
						return nil, err
					}

					resps = append(resps, resps_...)
				}
			}
		}
	case resourceTypeCalendarHomeSet:
		homeSetPath, err := b.Backend.CalendarHomeSetPath(r.Context())
		if err != nil {
			return nil, err
		}

		if samePath(r.URL.Path, homeSetPath) {
			resp, err := b.propFindHomeSet(r.Context(), propfind)
			if err != nil {
				return nil, err
			}

			resps = append(resps, *resp)

			if depth != internal.DepthZero {
				recurse := depth == internal.DepthInfinity

				resps_, err := b.propFindAllCalendars(r.Context(), propfind, recurse)
				if err != nil {
					return nil, err
				}

				resps = append(resps, resps_...)
			}
		}
	case resourceTypeCalendar:
		ab, err := b.Backend.GetCalendar(r.Context(), r.URL.Path)
		if err != nil {
			return nil, err
		}

		resp, err := b.propFindCalendar(r.Context(), propfind, ab)
		if err != nil {
			return nil, err
		}

		resps = append(resps, *resp)

		if depth != internal.DepthZero {
			resps_, err := b.propFindAllCalendarObjects(r.Context(), propfind, ab)
			if err != nil {
				return nil, err
			}

			resps = append(resps, resps_...)
		}
	case resourceTypeCalendarObject:
		ao, _, err := b.Backend.GetCalendarObject(r.Context(), r.URL.Path, &dataReq, webdav.Preconditions{})
		if err != nil {
			return nil, err
		}

		resp, err := b.propFindCalendarObject(r.Context(), propfind, ao)
		if err != nil {
			return nil, err
		}

		resps = append(resps, *resp)
	}

	return internal.NewMultiStatus(resps...), nil
}

func (b *backend) PropPatch(r *http.Request, update *internal.PropertyUpdate) (*internal.Response, error) {
	if b.resourceTypeAtPath(r.URL.Path) != resourceTypeCalendar {
		return nil, daverr.HTTPErrorf(http.StatusForbidden, "caldav: proppatch not allowed at given location")
	}

	cal, err := b.Backend.GetCalendar(r.Context(), r.URL.Path)
	if err != nil {
		return nil, err
	}

	protected, dependent := partitionProperties(update)
	if len(protected) > 0 {
		resp := &internal.Response{Hrefs: []internal.Href{{Path: r.URL.Path}}}
		if err := encodeProps(resp, http.StatusForbidden, protected); err != nil {
			return nil, err
		}

		if err := encodeProps(resp, http.StatusFailedDependency, dependent); err != nil {
			return nil, err
		}

		return resp, nil
	}

	names, err := applyPropertyUpdate(update, cal)
	if err != nil {
		return nil, err
	}

	if _, err := b.Backend.PutCalendar(r.Context(), cal); err != nil {
		return nil, err
	}

	resp := &internal.Response{Hrefs: []internal.Href{{Path: r.URL.Path}}}
	if err := encodeProps(resp, http.StatusOK, names); err != nil {
		return nil, err
	}

	return resp, nil
}

type unit struct{}

// protectedProperties lists the properties the server computes or controls.
var protectedProperties = map[xml.Name]unit{
	internal.ResourceTypeName:            {},
	internal.GetETagName:                 {},
	internal.GetContentLengthName:        {},
	internal.GetLastModifiedName:         {},
	internal.CreationDateName:            {},
	internal.SupportedReportSetName:      {},
	internal.CurrentUserPrincipalName:    {},
	internal.CurrentUserPrivilegeSetName: {},
	supportedCalendarDataName:            {},
	supportedCalendarComponentSetName:    {},
	maxResourceSizeName:                  {},
}

func isProtectedProperty(name xml.Name) bool {
	_, ok := protectedProperties[name]
	return ok
}

func hasProtectedProperty(props []internal.RawXMLValue) bool {
	for i := range props {
		if name, ok := props[i].XMLName(); ok && isProtectedProperty(name) {
			return true
		}
	}

	return false
}

func requestedProperties(update *internal.PropertyUpdate) []xml.Name {
	var names []xml.Name

	for _, set := range update.Set {
		for i := range set.Prop.Raw {
			name, ok := set.Prop.Raw[i].XMLName()
			if !ok {
				continue
			}

			names = append(names, name)
		}
	}

	for _, remove := range update.Remove {
		for i := range remove.Prop.Raw {
			name, ok := remove.Prop.Raw[i].XMLName()
			if !ok {
				continue
			}

			names = append(names, name)
		}
	}

	return names
}

// partitionProperties splits the requested properties into protected
// properties and dependent properties.
func partitionProperties(update *internal.PropertyUpdate) (protected, dependent []xml.Name) {
	for _, name := range requestedProperties(update) {
		if isProtectedProperty(name) {
			protected = append(protected, name)
		} else {
			dependent = append(dependent, name)
		}
	}

	return protected, dependent
}

func encodeProps(resp *internal.Response, status int, names []xml.Name) error {
	for _, name := range names {
		if err := resp.EncodeProp(status, internal.NewRawXMLElement(name, nil, nil)); err != nil {
			return err
		}
	}

	return nil
}

// applyPropertyUpdate applies the set and remove operations to the calendar.
func applyPropertyUpdate(update *internal.PropertyUpdate, cal *Calendar) ([]xml.Name, error) {
	if len(update.Set) > 0 && cal.DeadProperties == nil {
		cal.DeadProperties = map[xml.Name]string{}
	}

	setNames, err := applySetProperties(update.Set, cal)
	if err != nil {
		return nil, err
	}

	return append(setNames, applyRemoveProperties(update.Remove, cal)...), nil
}

func applySetProperties(sets []internal.Set, cal *Calendar) ([]xml.Name, error) {
	var names []xml.Name

	for _, set := range sets {
		for i := range set.Prop.Raw {
			raw := &set.Prop.Raw[i]

			name, ok := raw.XMLName()
			if !ok {
				continue
			}

			live, err := setLiveProperty(raw, name, cal)
			if err != nil {
				return nil, err
			}

			if !live {
				value, err := marshalProperty(raw)
				if err != nil {
					return nil, err
				}

				cal.DeadProperties[name] = value
			}

			names = append(names, name)
		}
	}

	return names, nil
}

func applyRemoveProperties(removes []internal.Remove, cal *Calendar) []xml.Name {
	var names []xml.Name

	for _, remove := range removes {
		for i := range remove.Prop.Raw {
			name, ok := remove.Prop.Raw[i].XMLName()
			if !ok {
				continue
			}

			if !clearLiveProperty(name, cal) {
				delete(cal.DeadProperties, name)
			}

			names = append(names, name)
		}
	}

	return names
}

func marshalProperty(raw *internal.RawXMLValue) (string, error) {
	data, err := xml.Marshal(raw)
	if err != nil {
		return "", fmt.Errorf("caldav: failed to encode property: %w", err)
	}

	return string(data), nil
}

// storeDeadProperties stores the properties of a MKCALENDAR request that the
// server does not handle directly.
func storeDeadProperties(props []internal.RawXMLValue, cal *Calendar) error {
	for i := range props {
		raw := &props[i]

		name, ok := raw.XMLName()
		if !ok {
			continue
		}

		value, err := marshalProperty(raw)
		if err != nil {
			return err
		}

		if cal.DeadProperties == nil {
			cal.DeadProperties = map[xml.Name]string{}
		}

		cal.DeadProperties[name] = value
	}

	return nil
}

func unmarshalProperty(value string) (*internal.RawXMLValue, error) {
	var raw internal.RawXMLValue
	// The decoder only accepts XML and returns an error for malformed input.
	if err := xml.Unmarshal([]byte(value), &raw); err != nil { //nolint:gosec
		return nil, fmt.Errorf("caldav: failed to decode property: %w", err)
	}

	return &raw, nil
}

// setLiveProperty stores a known live property in its typed calendar field.
// It reports whether the property is a known live property.
func setLiveProperty(raw *internal.RawXMLValue, name xml.Name, cal *Calendar) (bool, error) {
	switch name {
	case internal.DisplayNameName:
		var displayName internal.DisplayName
		if err := raw.Decode(&displayName); err != nil {
			return true, fmt.Errorf("caldav: failed to decode displayname: %w", err)
		}

		cal.Name = displayName.Name
	case calendarDescriptionName:
		var description calendarDescription
		if err := raw.Decode(&description); err != nil {
			return true, fmt.Errorf("caldav: failed to decode calendar-description: %w", err)
		}

		cal.Description = description.Description
	case calendarColorName:
		var color calendarColor
		if err := raw.Decode(&color); err != nil {
			return true, fmt.Errorf("caldav: failed to decode calendar-color: %w", err)
		}

		cal.Color = color.Color
	default:
		return false, nil
	}

	return true, nil
}

// clearLiveProperty clears the typed field of a known live property.
// It reports whether the property is a known live property.
func clearLiveProperty(name xml.Name, cal *Calendar) bool {
	switch name {
	case internal.DisplayNameName:
		cal.Name = ""
	case calendarDescriptionName:
		cal.Description = ""
	case calendarColorName:
		cal.Color = ""
	default:
		return false
	}

	return true
}

func mapPreconditionError(err error) error {
	if errors.Is(err, webdav.ErrPreconditionFailed) {
		return &daverr.HTTPError{Code: http.StatusPreconditionFailed, Err: err}
	}

	return err
}

//nolint:gocyclo
func (b *backend) Put(w http.ResponseWriter, r *http.Request) (err error) {
	t, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		return daverr.HTTPErrorf(http.StatusBadRequest, "caldav: malformed Content-Type: %v", err)
	}

	if t != MIMEType {
		return NewPreconditionError(PreconditionSupportedCalendarData)
	}

	// TODO: check CALDAV:max-resource-size precondition
	defer func() {
		if e := r.Body.Close(); e != nil {
			err = errors.Join(err, fmt.Errorf("error while closing request body: %w", e))
		}
	}()

	data, err := io.ReadAll(r.Body)
	if err != nil {
		return daverr.HTTPErrorf(http.StatusBadRequest, "caldav: failed to read request body: %v", err)
	}

	cal, err := ics.ParseCalendar(bytes.NewReader(data))
	if err != nil {
		return NewPreconditionError(PreconditionValidCalendarData)
	}

	if len(cal.Components) == 0 {
		return NewPreconditionError(PreconditionValidCalendarData)
	}

	pre, err := webdav.PreconditionsFromHeader(r.Header)
	if err != nil {
		return err
	}

	co, created, err := b.Backend.PutCalendarObject(r.Context(), r.URL.Path, cal, pre)
	if err != nil {
		return mapPreconditionError(err)
	}

	// A strong ETag is only valid when the stored representation is equal to
	// the submitted representation. Omit it when the server changed the bytes.
	stored := bytes.Buffer{}
	if co.Data != nil {
		if err := co.Data.SerializeTo(&stored); err != nil {
			return fmt.Errorf("caldav: failed to serialize calendar object: %w", err)
		}
	}

	if co.ETag != "" && bytes.Equal(data, stored.Bytes()) {
		w.Header().Set("ETag", internal.ETag(co.ETag).String())
	}

	if !co.ModTime.IsZero() {
		w.Header().Set("Last-Modified", co.ModTime.UTC().Format(http.TimeFormat))
	}

	if created {
		if co.Path != "" {
			w.Header().Set("Location", co.Path)
		}

		w.WriteHeader(http.StatusCreated)
	} else {
		w.WriteHeader(http.StatusNoContent)
	}

	return nil
}

func (b *backend) Delete(r *http.Request) error {
	pre, err := webdav.PreconditionsFromHeader(r.Header)
	if err != nil {
		return err
	}

	return mapPreconditionError(b.Backend.DeleteCalendarObject(r.Context(), r.URL.Path, pre))
}

func (b *backend) Mkcol(r *http.Request) error {
	if b.resourceTypeAtPath(r.URL.Path) != resourceTypeCalendar {
		return daverr.HTTPErrorf(http.StatusForbidden, "caldav: calendar creation not allowed at given location")
	}

	ctx := r.Context()
	cal := Calendar{
		Path: r.URL.Path,
	}

	if !internal.IsRequestBodyEmpty(r) {
		var m mkcolReq
		if err := internal.DecodeXMLRequest(r, &m); err != nil {
			return daverr.HTTPErrorf(http.StatusBadRequest, "carddav: error parsing mkcol request: %s", err.Error())
		}

		if !m.ResourceType.Is(internal.CollectionName) || !m.ResourceType.Is(calendarName) {
			return daverr.HTTPErrorf(http.StatusBadRequest, "carddav: unexpected resource type")
		}

		cal.Name = m.DisplayName
		cal.Description = m.Description
		cal.MaxResourceSize = m.MaxResourceSize
		cal.SupportedComponentSet = componentNames(m.SupportedComponents)
	}

	homeSetPath, err := b.Backend.CalendarHomeSetPath(ctx)
	if err != nil {
		return err
	}

	if !strings.HasPrefix(cal.Path, homeSetPath) {
		return ErrInvalidCalendarPath
	}

	_, err = b.Backend.PutCalendar(ctx, &cal)

	return err
}

func (b *backend) Mkcalendar(r *http.Request) error {
	if b.resourceTypeAtPath(r.URL.Path) != resourceTypeCalendar {
		return NewPreconditionError(PreconditionCalendarCollectionLocationOk)
	}

	ctx := r.Context()

	if _, err := b.Backend.GetCalendar(ctx, r.URL.Path); err == nil {
		return newDAVError(
			http.StatusForbidden,
			xml.Name{Space: internal.Namespace, Local: "resource-must-be-null"},
		)
	} else if !daverr.IsNotFound(err) {
		return err
	}

	cal := Calendar{
		Path: r.URL.Path,
	}

	if !internal.IsRequestBodyEmpty(r) {
		var m mkcalendarReq
		if err := internal.DecodeXMLRequest(r, &m); err != nil {
			return daverr.HTTPErrorf(
				http.StatusBadRequest,
				"carddav: error parsing mkcalendar request: %s",
				err.Error(),
			)
		}

		if hasProtectedProperty(m.Prop.Raw) {
			return daverr.HTTPErrorf(http.StatusForbidden, "caldav: cannot set protected property")
		}

		cal.Name = m.Prop.DisplayName
		cal.Description = m.Prop.Description
		cal.MaxResourceSize = m.Prop.MaxResourceSize
		cal.SupportedComponentSet = componentNames(m.Prop.SupportedComponents)

		if err := storeDeadProperties(m.Prop.Raw, &cal); err != nil {
			return err
		}
	}

	homeSetPath, err := b.Backend.CalendarHomeSetPath(ctx)
	if err != nil {
		return err
	}

	if !strings.HasPrefix(cal.Path, homeSetPath) {
		return ErrInvalidCalendarPath
	}

	_, err = b.Backend.PutCalendar(ctx, &cal)

	return err
}

func (b *backend) Copy(r *http.Request, dest *internal.Href, recursive, overwrite bool) (created bool, err error) {
	return false, daverr.HTTPErrorf(http.StatusNotImplemented, "caldav: Copy not implemented")
}

func (b *backend) Move(r *http.Request, dest *internal.Href, overwrite bool) (created bool, err error) {
	return false, daverr.HTTPErrorf(http.StatusNotImplemented, "caldav: Move not implemented")
}

func (b *backend) resourceTypeAtPath(reqPath string) resourceType {
	p := path.Clean(reqPath)

	p = strings.TrimPrefix(p, b.Prefix)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}

	if p == "/" {
		return resourceTypeRoot
	}

	return resourceType(len(strings.Split(p, "/")) - 1)
}

// samePath reports whether two request paths identify the same resource,
// ignoring trailing slashes.
func samePath(a, b string) bool {
	return path.Clean(a) == path.Clean(b)
}

func (b *backend) propFindRoot(ctx context.Context, propfind *internal.PropFind) (*internal.Response, error) {
	principalPath, err := b.Backend.CurrentUserPrincipal(ctx)
	if err != nil {
		return nil, err
	}

	props := map[xml.Name]internal.PropFindFunc{
		internal.CurrentUserPrincipalName: internal.PropFindValue(&internal.CurrentUserPrincipal{
			Href: internal.Href{Path: principalPath},
		}),
		internal.ResourceTypeName: internal.PropFindValue(internal.NewResourceType(internal.CollectionName)),
	}

	return internal.NewPropFindResponse(principalPath, propfind, props)
}

func (b *backend) propFindUserPrincipal(ctx context.Context, propfind *internal.PropFind) (*internal.Response, error) {
	principalPath, err := b.Backend.CurrentUserPrincipal(ctx)
	if err != nil {
		return nil, err
	}

	homeSetPath, err := b.Backend.CalendarHomeSetPath(ctx)
	if err != nil {
		return nil, err
	}

	props := map[xml.Name]internal.PropFindFunc{
		internal.CurrentUserPrincipalName: internal.PropFindValue(&internal.CurrentUserPrincipal{
			Href: internal.Href{Path: principalPath},
		}),
		calendarHomeSetName: internal.PropFindValue(&calendarHomeSet{
			Href: internal.Href{Path: homeSetPath},
		}),
		internal.ResourceTypeName: internal.PropFindValue(
			internal.NewResourceType(internal.CollectionName, internal.PrincipalName),
		),
	}

	return internal.NewPropFindResponse(principalPath, propfind, props)
}

func (b *backend) propFindHomeSet(ctx context.Context, propfind *internal.PropFind) (*internal.Response, error) {
	principalPath, err := b.Backend.CurrentUserPrincipal(ctx)
	if err != nil {
		return nil, err
	}

	homeSetPath, err := b.Backend.CalendarHomeSetPath(ctx)
	if err != nil {
		return nil, err
	}

	// TODO anything else to return here?
	props := map[xml.Name]internal.PropFindFunc{
		internal.CurrentUserPrincipalName: internal.PropFindValue(&internal.CurrentUserPrincipal{
			Href: internal.Href{Path: principalPath},
		}),
		internal.ResourceTypeName: internal.PropFindValue(internal.NewResourceType(internal.CollectionName)),
	}

	return internal.NewPropFindResponse(homeSetPath, propfind, props)
}

func (b *backend) propFindCalendar(
	ctx context.Context,
	propfind *internal.PropFind,
	cal *Calendar,
) (*internal.Response, error) {
	props := map[xml.Name]internal.PropFindFunc{
		internal.CurrentUserPrincipalName: func(*internal.RawXMLValue) (any, error) {
			path, err := b.Backend.CurrentUserPrincipal(ctx)
			if err != nil {
				return nil, err
			}

			return &internal.CurrentUserPrincipal{Href: internal.Href{Path: path}}, nil
		},
		internal.ResourceTypeName: internal.PropFindValue(
			internal.NewResourceType(internal.CollectionName, calendarName),
		),
		internal.SupportedReportSetName: internal.PropFindValue(
			internal.NewSupportedReportSet(calendarQueryName, calendarMultigetName, freeBusyQueryName),
		),
		supportedCalendarDataName: internal.PropFindValue(&supportedCalendarData{
			Types: []calendarDataType{
				{ContentType: MIMEType, Version: "2.0"},
			},
		}),
		supportedCalendarComponentSetName: func(*internal.RawXMLValue) (any, error) {
			components := []comp{}

			if cal.SupportedComponentSet != nil {
				for _, name := range cal.SupportedComponentSet {
					components = append(components, comp{Name: name})
				}
			} else {
				components = append(components, comp{Name: string(ics.ComponentVEvent)})
			}

			return &supportedCalendarComponentSet{
				Comp: components,
			}, nil
		},
		// TODO: Gather UserPrivilege from backend to control read-and-write
		internal.CurrentUserPrivilegeSetName: internal.PropFindValue(&internal.CurrentUserPrivilegeSet{
			Privilege: []internal.Privilege{{
				Read:  &struct{}{},
				Write: &struct{}{},
			}},
		}),
	}

	if cal.Name != "" {
		props[internal.DisplayNameName] = internal.PropFindValue(&internal.DisplayName{
			Name: cal.Name,
		})
	}

	if cal.Description != "" {
		props[calendarDescriptionName] = internal.PropFindValue(&calendarDescription{
			Description: cal.Description,
		})
	}

	if cal.MaxResourceSize > 0 {
		props[maxResourceSizeName] = internal.PropFindValue(&maxResourceSize{
			Size: cal.MaxResourceSize,
		})
	}

	if cal.Color != "" {
		props[calendarColorName] = internal.PropFindValue(&calendarColor{
			Color: cal.Color,
		})
	}

	for name, value := range cal.DeadProperties {
		if _, exists := props[name]; exists {
			continue
		}

		props[name] = func(*internal.RawXMLValue) (any, error) {
			return unmarshalProperty(value)
		}
	}

	// TODO: CALDAV:min-date-time, CALDAV:max-date-time,
	// CALDAV:max-instances, CALDAV:max-attendees-per-instance

	return internal.NewPropFindResponse(cal.Path, propfind, props)
}

func (b *backend) propFindAllCalendars(
	ctx context.Context,
	propfind *internal.PropFind,
	recurse bool,
) ([]internal.Response, error) {
	abs, err := b.Backend.ListCalendars(ctx)
	if err != nil {
		return nil, err
	}

	var resps []internal.Response

	for _, ab := range abs {
		resp, err := b.propFindCalendar(ctx, propfind, &ab)
		if err != nil {
			return nil, err
		}

		resps = append(resps, *resp)

		if recurse {
			resps_, err := b.propFindAllCalendarObjects(ctx, propfind, &ab)
			if err != nil {
				return nil, err
			}

			resps = append(resps, resps_...)
		}
	}

	return resps, nil
}

func (b *backend) propFindCalendarObject(
	ctx context.Context,
	propfind *internal.PropFind,
	co *CalendarObject,
) (*internal.Response, error) {
	props := map[xml.Name]internal.PropFindFunc{
		internal.CurrentUserPrincipalName: func(*internal.RawXMLValue) (any, error) {
			path, err := b.Backend.CurrentUserPrincipal(ctx)
			if err != nil {
				return nil, err
			}

			return &internal.CurrentUserPrincipal{Href: internal.Href{Path: path}}, nil
		},
		internal.GetContentTypeName: internal.PropFindValue(&internal.GetContentType{
			Type: MIMEType,
		}),
		internal.SupportedReportSetName: internal.PropFindValue(
			internal.NewSupportedReportSet(calendarQueryName, calendarMultigetName),
		),
		// TODO: calendar-data can only be used in REPORT requests
		calendarDataName: func(*internal.RawXMLValue) (any, error) {
			var buf bytes.Buffer
			if err := co.Data.SerializeTo(&buf); err != nil {
				return nil, err
			}

			return &calendarDataResp{Data: buf.Bytes()}, nil
		},
	}

	if co.ContentLength > 0 {
		props[internal.GetContentLengthName] = internal.PropFindValue(&internal.GetContentLength{
			Length: co.ContentLength,
		})
	}

	if !co.ModTime.IsZero() {
		props[internal.GetLastModifiedName] = internal.PropFindValue(&internal.GetLastModified{
			LastModified: internal.Time(co.ModTime),
		})
	}

	if co.ETag != "" {
		props[internal.GetETagName] = internal.PropFindValue(&internal.GetETag{
			ETag: internal.ETag(co.ETag),
		})
	}

	return internal.NewPropFindResponse(co.Path, propfind, props)
}

func (b *backend) propFindAllCalendarObjects(
	ctx context.Context,
	propfind *internal.PropFind,
	cal *Calendar,
) ([]internal.Response, error) {
	var dataReq CalendarCompRequest

	aos, err := b.Backend.ListCalendarObjects(ctx, cal.Path, &dataReq)
	if err != nil {
		return nil, err
	}

	var resps []internal.Response

	for _, ao := range aos {
		resp, err := b.propFindCalendarObject(ctx, propfind, &ao)
		if err != nil {
			return nil, err
		}

		resps = append(resps, *resp)
	}

	return resps, nil
}

// PreconditionType implements https://datatracker.ietf.org/doc/html/rfc4791#section-5.3.2.1
type PreconditionType string

const (
	PreconditionNoUIDConflict                PreconditionType = "no-uid-conflict"
	PreconditionSupportedCalendarData        PreconditionType = "supported-calendar-data"
	PreconditionSupportedCalendarComponent   PreconditionType = "supported-calendar-component"
	PreconditionValidCalendarData            PreconditionType = "valid-calendar-data"
	PreconditionValidCalendarObjectResource  PreconditionType = "valid-calendar-object-resource"
	PreconditionCalendarCollectionLocationOk PreconditionType = "calendar-collection-location-ok"
	PreconditionMaxResourceSize              PreconditionType = "max-resource-size"
	PreconditionMinDateTime                  PreconditionType = "min-date-time"
	PreconditionMaxDateTime                  PreconditionType = "max-date-time"
	PreconditionMaxInstances                 PreconditionType = "max-instances"
	PreconditionMaxAttendeesPerInstance      PreconditionType = "max-attendees-per-instance"
)

// NewPreconditionError returns a method error for a CalDAV precondition.
func NewPreconditionError(precondition PreconditionType) error {
	code := http.StatusForbidden
	if precondition == PreconditionNoUIDConflict {
		code = http.StatusConflict
	}

	return newDAVError(code, xml.Name{Space: caldavNS, Local: string(precondition)})
}

func newDAVError(code int, names ...xml.Name) error {
	raw := make([]internal.RawXMLValue, len(names))
	for i, name := range names {
		raw[i] = *internal.NewRawXMLElement(name, nil, nil)
	}

	return &daverr.HTTPError{
		Code: code,
		Err: &internal.Error{
			Raw: raw,
		},
	}
}
