package store

import (
	"bufio"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

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

func prefixEnd(path Path) Path {
	return Path(fmt.Sprintf("%s%c", path, utf8.MaxRune))
}

type Calendar struct {
	Path     Path
	Metadata CalendarMetadata
}

// DeadProperties holds client-defined WebDAV properties stored as opaque XML,
// unlike live properties that the server computes or enforces. Known
// properties with server behavior stay as typed fields.
type DeadProperties map[xml.Name]string

type calendarProperty struct {
	Name  xml.Name `json:"name"`
	Value string   `json:"value"`
}

// MarshalJSON implements json.Marshaler.
func (p DeadProperties) MarshalJSON() ([]byte, error) {
	properties := make([]calendarProperty, 0, len(p))
	for name, value := range p {
		properties = append(properties, calendarProperty{Name: name, Value: value})
	}

	return json.Marshal(properties)
}

// UnmarshalJSON implements json.Unmarshaler.
func (p *DeadProperties) UnmarshalJSON(data []byte) error {
	var properties []calendarProperty
	if err := json.Unmarshal(data, &properties); err != nil {
		return err
	}

	*p = make(DeadProperties, len(properties))
	for _, property := range properties {
		(*p)[property.Name] = property.Value
	}

	return nil
}

//nolint:tagliatelle
type CalendarMetadata struct {
	Name                  string         `json:"name,omitempty"`
	Description           string         `json:"description,omitempty"`
	SupportedComponentSet []string       `json:"supported-component-set,omitempty"`
	MaxResourceSize       int64          `json:"max-resource-size,omitempty"`
	Color                 string         `json:"color,omitempty"`
	Tag                   string         `json:"tag,omitempty"`
	DeadProperties        DeadProperties `json:"dead-properties,omitempty"`
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
