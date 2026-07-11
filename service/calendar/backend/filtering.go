package backend

import (
	"maps"
	"time"

	"github.com/emersion/go-ical"

	"github.com/teapotovh/teapot/lib/webdav/caldav"
)

func mapCalendarObject(object *caldav.CalendarObject, req *caldav.CalendarCompRequest) (*caldav.CalendarObject, error) {
	if req == nil {
		return object, nil
	}

	if object.Data == nil {
		return object, nil
	}

	if err := filterComponent(object.Data.Component, req); err != nil {
		return nil, err
	}

	return object, nil
}

//nolint:gocyclo
func filterComponent(comp *ical.Component, req *caldav.CalendarCompRequest) error {
	//
	// Filter properties
	//
	if !req.AllProps {
		// Keep all
	} else if len(req.Props) > 0 {
		wanted := make(map[string]struct{}, len(req.Props))
		for _, p := range req.Props {
			wanted[p] = struct{}{}
		}

		for name := range comp.Props {
			if _, ok := wanted[name]; !ok {
				delete(comp.Props, name)
			}
		}
	} else {
		// No props requested
		for name := range comp.Props {
			delete(comp.Props, name)
		}
	}

	//
	// Filter and recurse into sub-components
	//
	if req.AllComps {
		snapshot := append([]*ical.Component{}, comp.Children...)
		for _, child := range snapshot {
			if err := applyExpand(comp, child, req.Expand); err != nil {
				return err
			}
		}
		// Recurse into each child with a permissive request
		passthroughReq := &caldav.CalendarCompRequest{AllProps: true, AllComps: true}
		for _, child := range comp.Children {
			if err := filterComponent(child, passthroughReq); err != nil {
				return err
			}
		}
	} else if len(req.Comps) > 0 {
		subReqs := make(map[string]*caldav.CalendarCompRequest, len(req.Comps))
		for i := range req.Comps {
			sr := req.Comps[i]
			subReqs[sr.Name] = &sr
		}

		snapshot := append([]*ical.Component{}, comp.Children...)
		for _, child := range snapshot {
			sr, ok := subReqs[child.Name]
			if !ok {
				continue
			}

			if err := applyExpand(comp, child, sr.Expand); err != nil {
				return err
			}
		}

		filtered := comp.Children[:0]
		for _, child := range comp.Children {
			sr, ok := subReqs[child.Name]
			if !ok {
				continue
			}

			if err := filterComponent(child, sr); err != nil {
				return err
			}

			filtered = append(filtered, child)
		}

		comp.Children = filtered
	} else {
		comp.Children = nil
	}

	return nil
}

// applyExpand expands a recurring child component into individual instances
// within the requested time range, updating parent.Children in place.
// For non-recurring components it is a no-op.
func applyExpand(parent *ical.Component, comp *ical.Component, expand *caldav.CalendarExpandRequest) error {
	if expand == nil {
		return nil
	}

	switch comp.Name {
	case ical.CompEvent, ical.CompToDo, ical.CompJournal, ical.CompFreeBusy:
	default:
		return nil
	}

	set, err := comp.RecurrenceSet(time.UTC)
	if err != nil || set == nil {
		return nil //nolint:nilerr
	}

	occurrences := set.Between(expand.Start, expand.End, true)

	// Replace the single recurring component with expanded instances
	newChildren := make([]*ical.Component, 0, len(parent.Children)-1+len(occurrences))
	for _, child := range parent.Children {
		if child != comp {
			newChildren = append(newChildren, child)
		}
	}

	for _, t := range occurrences {
		instance := &ical.Component{
			Name:     comp.Name,
			Props:    make(ical.Props),
			Children: comp.Children,
		}
		maps.Copy(instance.Props, comp.Props)

		dtProp := ical.NewProp(ical.PropDateTimeStart)
		dtProp.SetDateTime(t)
		instance.Props[ical.PropDateTimeStart] = []ical.Prop{*dtProp}
		delete(instance.Props, ical.PropRecurrenceRule)
		newChildren = append(newChildren, instance)
	}

	parent.Children = newChildren

	return nil
}
