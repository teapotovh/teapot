package store

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/emersion/go-ical"

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
	Path        Path
	Name        string
	Description string
}

// Key implements pgcache.Object.
func (c Calendar) Key() Path {
	return c.Path
}

type Object struct {
	Path    Path
	ModTime time.Time
	Data    []byte
}

// Key implements pgcache.Object.
func (o Object) Key() Path {
	return o.Path
}

func (o *Object) Size() int64 {
	return int64(len(o.Data))
}

func (o *Object) ETag() string {
	sum := sha1.Sum(o.Data)
	return base64.StdEncoding.EncodeToString(sum[:])
}

func (o *Object) Calendar() (*ical.Calendar, error) {
	reader := bytes.NewReader(o.Data)
	decoder := ical.NewDecoder(reader)

	cal, err := decoder.Decode()
	if err != nil {
		return nil, fmt.Errorf("error while decoding ical: %w", err)
	}

	return cal, nil
}

func SerializeObject(obj caldav.CalendarObject) (*Object, error) {
	buf := bytes.Buffer{}
	writer := bufio.NewWriter(&buf)
	encoder := ical.NewEncoder(writer)

	if err := encoder.Encode(obj.Data); err != nil {
		return nil, fmt.Errorf("error while encoding ical: %w", err)
	}

	if err := writer.Flush(); err != nil {
		return nil, fmt.Errorf("error while flushing encoder buffer: %w", err)
	}

	so := Object{
		Path:    Path(obj.Path),
		ModTime: obj.ModTime,
		Data:    buf.Bytes(),
	}

	return &so, nil
}
