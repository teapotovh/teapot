package log

import (
	"bytes"
	"io"
)

type flushBuffer struct {
	buf          bytes.Buffer
	bytesWritten uint64
	w            io.Writer
}

func newFlushBuffer(w io.Writer) *flushBuffer {
	return &flushBuffer{w: w}
}

func (f *flushBuffer) Write(p []byte) (int, error) {
	f.bytesWritten += uint64(len(p))
	return f.buf.Write(p)
}

func (f *flushBuffer) Flush() error {
	_, err := f.buf.WriteTo(f.w)
	return err
}

func (f *flushBuffer) Position() uint64 {
	return f.bytesWritten
}
