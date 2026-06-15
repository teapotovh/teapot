package store

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/emersion/go-ical"

	"github.com/teapotovh/teapot/lib/webdav/caldav"
)

type Calendar struct {
	Path        string
	Name        string
	Description string
}

type Object struct {
	Path    string
	ModTime time.Time
	Data    []byte
}

func (o *Object) Size() int64 {
	return int64(len(o.Data))
}

func (o *Object) CalAndETag() (*ical.Calendar, string, error) {
	reader := bytes.NewReader(o.Data)
	decoder := ical.NewDecoder(reader)

	cal, err := decoder.Decode()
	if err != nil {
		return nil, "", fmt.Errorf("error while decoding ical: %w", err)
	}

	sum := sha1.Sum(o.Data)
	etag := base64.StdEncoding.EncodeToString(sum[:])

	return cal, etag, nil
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
		Path:    obj.Path,
		ModTime: obj.ModTime,
		Data:    buf.Bytes(),
	}

	return &so, nil
}
