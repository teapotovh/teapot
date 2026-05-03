package log

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

var (
	ErrBytesMismatch = errors.New("bytes mismatch")

	LogFileMode = os.FileMode(0o644)
)

type unit struct{}

type worker struct {
	logger  *slog.Logger
	context context.Context

	logDirectory string
	file         *os.File
	buffered     *bufio.Writer
	request      chan workerRequest
	stopped      chan unit
}

type workerRequest struct {
	data   json.RawMessage
	result chan error
}

func newWorker(ctx context.Context, logDirectory string, capacity uint32, logger *slog.Logger) (*worker, error) {
	logPath := filepath.Join(logDirectory, "latest.log") // TODO: log rotation
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, LogFileMode)
	if err != nil {
		return nil, fmt.Errorf("error while opening log file at %q: %w", logPath, err)
	}

	logger.Info("path", "path", logDirectory, "file", logPath)

	buffered := bufio.NewWriter(file)
	w := worker{
		logger:  logger,
		context: ctx,

		file:     file,
		buffered: buffered,
		request:  make(chan workerRequest, capacity),
		stopped:  make(chan unit, 1),
	}

	return &w, nil
}

func (w *worker) run() {
	for {
		select {
		case <-w.context.Done():
			return

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
				err = fmt.Errorf("error while writing log line to disk: %w", err)
				req.result <- ErrBytesMismatch
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
