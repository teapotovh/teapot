package store

import (
	"bufio"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"maps"
	"strings"
	"time"

	ics "github.com/arran4/golang-ical"

	"github.com/teapotovh/teapot/lib/s3cache"
	"github.com/teapotovh/teapot/lib/webdav/caldav"
)

type Path string

func PrefixFromString(rawPath string) (*Path, error) {
	path := Path(rawPath)
	return &path, nil
}

func (path Path) Less(p Path) bool {
	return strings.Compare(string(path), string(p)) == -1
}

func (path Path) String() string {
	return string(path)
}

type Calendar struct {
	Path     Path
	Metadata CalendarMetadata
}

// CalendarProperties stores full XML values for calendar properties.
type CalendarProperties = map[xml.Name]string

type calendarProperty struct {
	Name  xml.Name `json:"name"`
	Value string   `json:"value"`
}

type calendarMetadata struct {
	Properties []calendarProperty `json:"properties"`
}

type CalendarMetadata struct {
	Properties CalendarProperties `json:"-"`
}

// MarshalJSON implements json.Marshaler.
func (m CalendarMetadata) MarshalJSON() ([]byte, error) {
	properties := make([]calendarProperty, 0, len(m.Properties))
	for name, value := range m.Properties {
		properties = append(properties, calendarProperty{Name: name, Value: value})
	}

	return json.Marshal(calendarMetadata{Properties: properties})
}

// UnmarshalJSON implements json.Unmarshaler.
func (m *CalendarMetadata) UnmarshalJSON(data []byte) error {
	var metadata calendarMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return err
	}

	m.Properties = make(CalendarProperties, len(metadata.Properties))
	for _, property := range metadata.Properties {
		m.Properties[property.Name] = property.Value
	}

	return nil
}

// Clone returns a copy that does not share the properties map.
func (m CalendarMetadata) Clone() CalendarMetadata {
	m.Properties = maps.Clone(m.Properties)

	return m
}

// Clone returns a copy that does not share the properties map.
func (c Calendar) Clone() Calendar {
	c.Metadata = c.Metadata.Clone()

	return c
}

// Key implements pgcache.Object.
func (c Calendar) Key() Path {
	return c.Path
}

type Object struct {
	Path    Path
	ModTime time.Time
	Data    []byte
	ETag    string
}

func (o *Object) Size() int64 {
	return int64(len(o.Data))
}

func (o *Object) Calendar() (*ics.Calendar, error) {
	reader := bytes.NewReader(o.Data)

	cal, err := ics.ParseCalendar(reader)
	if err != nil {
		return nil, fmt.Errorf("error while parsing ics: %w", err)
	}

	return cal, nil
}

func SerializeObject(obj caldav.CalendarObject) (*Object, error) {
	buf := bytes.Buffer{}
	writer := bufio.NewWriter(&buf)

	if err := obj.Data.SerializeTo(writer); err != nil {
		return nil, fmt.Errorf("error while encoding ical: %w", err)
	}

	if err := writer.Flush(); err != nil {
		return nil, fmt.Errorf("error while flushing encoder buffer: %w", err)
	}

	data := buf.Bytes()
	etag := s3cache.HashBytes(data)
	object := Object{
		Path:    Path(obj.Path),
		ModTime: obj.ModTime,
		Data:    data,
		ETag:    string(etag),
	}

	return &object, nil
}
