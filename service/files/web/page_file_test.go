package web

import (
	"bytes"
	"io"
	"testing"

	"github.com/teapotovh/teapot/service/files/store"
)

type memoryFile struct {
	*bytes.Reader
}

func (f *memoryFile) Close() error {
	return nil
}

func (f *memoryFile) Stat() (store.Inode, error) {
	return store.Inode{}, nil
}

func TestDetectContentType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content []byte
		want    string
	}{
		{
			name:    "html",
			content: []byte("<!DOCTYPE html><html></html>"),
			want:    "text/html",
		},
		{
			name:    "svg",
			content: []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`),
			want:    "image/svg+xml",
		},
		{
			name:    "png",
			content: []byte("\x89PNG\r\n\x1a\n"),
			want:    "image/png",
		},
		{
			name:    "text",
			content: []byte("hello"),
			want:    "text/plain",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			file := &memoryFile{bytes.NewReader(test.content)}

			got, err := detectContentType(file)
			if err != nil {
				t.Fatalf("detectContentType() error = %v", err)
			}

			if got != test.want {
				t.Errorf("detectContentType() = %q, want %q", got, test.want)
			}

			rest, err := io.ReadAll(file)
			if err != nil {
				t.Fatalf("could not read file after detection: %v", err)
			}

			if !bytes.Equal(rest, test.content) {
				t.Errorf("file content after detection = %q, want %q", rest, test.content)
			}
		})
	}
}
