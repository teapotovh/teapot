package log

import (
	"bytes"
	"io"
)

type flushBuffer struct {
	buf bytes.Buffer
	w   io.Writer
}

func newFlushBuffer(w io.Writer) *flushBuffer {
	return &flushBuffer{w: w}
}

func (f *flushBuffer) Write(p []byte) (int, error) {
	return f.buf.Write(p)
}

func (f *flushBuffer) Flush() error {
	_, err := f.buf.WriteTo(f.w)
	return err
}
