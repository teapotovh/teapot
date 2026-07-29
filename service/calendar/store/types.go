package store

import (
	"bufio"
	"bytes"
	"fmt"
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

//nolint:tagliatelle
type CalendarMetadata struct {
	Name                  string   `json:"name,omitempty"`
	Description           string   `json:"description,omitempty"`
	SupportedComponentSet []string `json:"supported-calendar-component-set,omitempty"`
	MaxResourceSize       int64    `json:"max-resource-size,omitempty"`
	Color                 string   `json:"color,omitempty"`
	Tag                   string   `json:"tag,omitempty"`
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
