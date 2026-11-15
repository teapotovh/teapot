package caldav

import (
	"encoding/xml"
	"fmt"
	"iter"
	"maps"

	"github.com/teapotovh/teapot/lib/webdav/internal"
)

// CalendarProperties stores full XML values for calendar properties.
type CalendarProperties struct {
	values map[xml.Name]string
}

// NewCalendarProperties returns properties with the given XML values.
func NewCalendarProperties(values map[xml.Name]string) CalendarProperties {
	return CalendarProperties{values: maps.Clone(values)}
}

// Values returns a copy of the XML values.
func (p CalendarProperties) Values() map[xml.Name]string {
	return maps.Clone(p.values)
}

// All iterates over all properties.
func (p CalendarProperties) All() iter.Seq2[xml.Name, string] {
	return func(yield func(xml.Name, string) bool) {
		for name, value := range p.values {
			if !yield(name, value) {
				return
			}
		}
	}
}

// Get returns the XML value for a property.
func (p CalendarProperties) Get(name xml.Name) string {
	return p.values[name]
}

// Set stores the XML value for a property.
func (p *CalendarProperties) Set(name xml.Name, value string) {
	if p.values == nil {
		p.values = make(map[xml.Name]string)
	}

	p.values[name] = value
}

// Remove removes a property.
func (p *CalendarProperties) Remove(name xml.Name) {
	delete(p.values, name)
}

// DisplayName returns the calendar display name.
func (p CalendarProperties) DisplayName() string {
	var value internal.DisplayName
	if _, err := p.decode(internal.DisplayNameName, &value); err != nil {
		return ""
	}

	return value.Name
}

// SetDisplayName stores the calendar display name.
func (p *CalendarProperties) SetDisplayName(value string) error {
	return p.encode(internal.DisplayNameName, &internal.DisplayName{Name: value})
}

// Description returns the calendar description.
func (p CalendarProperties) Description() string {
	var value calendarDescription
	if _, err := p.decode(calendarDescriptionName, &value); err != nil {
		return ""
	}

	return value.Description
}

// SetDescription stores the calendar description.
func (p *CalendarProperties) SetDescription(value string) error {
	return p.encode(calendarDescriptionName, &calendarDescription{Description: value})
}

// Color returns the calendar color.
func (p CalendarProperties) Color() string {
	var value calendarColor
	if _, err := p.decode(calendarColorName, &value); err != nil {
		return ""
	}

	return value.Color
}

// SetColor stores the calendar color.
func (p *CalendarProperties) SetColor(value string) error {
	return p.encode(calendarColorName, &calendarColor{Color: value})
}

// SupportedComponentSet returns the supported calendar components.
func (p CalendarProperties) SupportedComponentSet() ([]string, error) {
	var value supportedCalendarComponentSet
	if _, err := p.decode(supportedCalendarComponentSetName, &value); err != nil {
		return nil, err
	}

	if len(value.Comp) == 0 {
		return nil, nil
	}

	names := make([]string, 0, len(value.Comp))
	for _, component := range value.Comp {
		names = append(names, component.Name)
	}

	return names, nil
}

// SetSupportedComponentSet stores the supported calendar components.
func (p *CalendarProperties) SetSupportedComponentSet(names []string) error {
	value := supportedCalendarComponentSet{
		Comp: make([]comp, 0, len(names)),
	}
	for _, name := range names {
		value.Comp = append(value.Comp, comp{Name: name})
	}

	return p.encode(supportedCalendarComponentSetName, &value)
}

// MaxResourceSize returns the maximum resource size.
func (p CalendarProperties) MaxResourceSize() (int64, error) {
	var value maxResourceSize
	if _, err := p.decode(maxResourceSizeName, &value); err != nil {
		return 0, err
	}

	return value.Size, nil
}

// SetMaxResourceSize stores the maximum resource size.
func (p *CalendarProperties) SetMaxResourceSize(size int64) error {
	return p.encode(maxResourceSizeName, &maxResourceSize{Size: size})
}

func (p CalendarProperties) decode(name xml.Name, value any) (bool, error) {
	raw, ok := p.values[name]
	if !ok {
		return false, nil
	}

	if err := xml.Unmarshal([]byte(raw), value); err != nil {
		return true, fmt.Errorf("caldav: failed to decode %s: %w", name.Local, err)
	}

	return true, nil
}

func (p *CalendarProperties) encode(name xml.Name, value any) error {
	raw, err := xml.Marshal(value)
	if err != nil {
		return fmt.Errorf("caldav: failed to encode %s: %w", name.Local, err)
	}

	p.Set(name, string(raw))

	return nil
}
