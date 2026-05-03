package log

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

var (
	ErrBytesMismatch = errors.New("bytes mismatch")

	LogFileMode = os.FileMode(0o644)
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

type unit struct{}

type worker struct {
	logger  *slog.Logger
	context context.Context

	logDirectory  string
	file          *os.File
	flushInterval time.Duration
	buffered      *flushBuffer
	request       chan workerRequest
	stopped       chan unit
}

type workerRequest struct {
	data   json.RawMessage
	result chan error
}

func newWorker(ctx context.Context, logDirectory string, flushInterval time.Duration, capacity uint32, logger *slog.Logger) (*worker, error) {
	logPath := filepath.Join(logDirectory, "latest.log") // TODO: log rotation
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, LogFileMode)
	if err != nil {
		return nil, fmt.Errorf("error while opening log file at %q: %w", logPath, err)
	}

	logger.Info("path", "path", logDirectory, "file", logPath)

	buffered := newFlushBuffer(file)
	w := worker{
		logger:  logger,
		context: ctx,

		file:          file,
		flushInterval: flushInterval,
		buffered:      buffered,
		request:       make(chan workerRequest, capacity),
		stopped:       make(chan unit, 1),
	}

	return &w, nil
}

func (w *worker) run() {
	flushTicker := time.NewTicker(w.flushInterval)
	defer flushTicker.Stop()

	flush := make(chan unit)
	defer close(flush)

	linesWrittenSinceLastFlush := 0

	for {
		select {
		case <-w.context.Done():
			return

		case <-flushTicker.C:
		case <-flush:
			if err := w.buffered.Flush(); err != nil {
				w.logger.Error("error while flushing log buffer", "error", err)
			}

			linesWrittenSinceLastFlush = 0

		case req := <-w.request:
			w.logger.Debug("writing log", "bytes", len(req.data))

			if len(req.data) > 1 && req.data[len(req.data)-1] != '\n' {
				req.data = append(req.data, '\n')
			}

			l, err := w.buffered.Write(req.data)
			if err != nil {
				err = fmt.Errorf("error while writing log line to disk: %w", err)
				req.result <- err

				continue
			}

			if l != len(req.data) {
				req.result <- ErrBytesMismatch

				continue
			}

			linesWrittenSinceLastFlush++
			if linesWrittenSinceLastFlush > 100 {
				select {
				case flush <- unit{}:
				default:
				}
			}

			req.result <- nil
		}
	}
}

func (w *worker) stop() {
	<-w.stopped
	w.buffered.Flush() // TODO: errors
	w.file.Close()     // TODO: errors
}
