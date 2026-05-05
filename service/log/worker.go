package log

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	ErrBytesMismatch = errors.New("bytes mismatch")

	LogFileMode = os.FileMode(0o644)
)

const LatestLogFilename = "latest"

type unit struct{}

type worker struct {
	logger  *slog.Logger
	context context.Context

	source                  string
	logDirectory            string
	flushInterval           time.Duration
	maxLogLinesBeforeFlush  uint32
	rotateInterval          time.Duration
	maxFileSizeBeforeRotate uint64

	file     *os.File
	buffered *flushBuffer
	writer   *gzip.Writer
	request  chan workerRequest
	stopped  chan unit
	metrics  *metrics
}

type workerRequest struct {
	data       json.RawMessage
	result     chan error
	insertedAt time.Time
	level      string
}

func newWorker(
	ctx context.Context,
	source string,
	logDirectory string,
	flushInterval time.Duration,
	maxLogLinesBeforeFlush uint32,
	rotateInterval time.Duration,
	maxFileSizeBeforeRotate uint64,
	capacity uint32,
	metrics *metrics,
	logger *slog.Logger,
) (*worker, error) {
	w := worker{
		logger:  logger,
		context: ctx,

		source:                  source,
		logDirectory:            logDirectory,
		flushInterval:           flushInterval,
		maxLogLinesBeforeFlush:  maxLogLinesBeforeFlush,
		rotateInterval:          rotateInterval,
		maxFileSizeBeforeRotate: maxFileSizeBeforeRotate,

		request: make(chan workerRequest, capacity),
		stopped: make(chan unit, 1),
		metrics: metrics,
	}

	if err := w.openLogFile(); err != nil {
		return nil, err
	}

	return &w, nil
}

func (w *worker) logFilePath(name string) string {
	name = w.source + "-" + name + ".jsonl.gz"
	return filepath.Join(w.logDirectory, filepath.Clean(name))
}

func (w *worker) openLogFile() error {
	logPath := w.logFilePath(LatestLogFilename)

	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, LogFileMode) //nolint:gosec
	if err != nil {
		return fmt.Errorf("error while opening log file at %q: %w", logPath, err)
	}

	w.file = file
	w.buffered = newFlushBuffer(file)
	w.writer = gzip.NewWriter(w.buffered)

	return nil
}

func (w *worker) closeCurrentFile() error {
	if err := w.writer.Flush(); err != nil {
		return fmt.Errorf("error while flushing gzip writer: %w", err)
	}

	path := w.logFilePath(LatestLogFilename)
	if err := w.buffered.Flush(); err != nil {
		return fmt.Errorf("error while flushing current log file %q: %w", path, err)
	}

	if err := w.file.Close(); err != nil {
		return fmt.Errorf("error while closing current log file %q: %w", path, err)
	}

	w.file = nil
	w.writer = nil

	archivalPath := w.logFilePath(time.Now().Format(time.RFC3339))
	if _, err := os.Stat(archivalPath); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("attempted to archive file to already existing path: %q (%w)", archivalPath, err)
	}

	if err := os.Rename(path, archivalPath); err != nil {
		return fmt.Errorf("error while arching current log file to %q: %w", archivalPath, err)
	}

	w.logger.Info("rotated current log file to archival path", "path", archivalPath)

	return nil
}

//nolint:gocyclo
func (w *worker) run() {
	flush := newManualTicker(w.flushInterval)
	defer flush.Stop()

	rotate := newManualTicker(w.rotateInterval)
	defer rotate.Stop()

	linesWrittenSinceLastFlush := uint32(0)
	bytesWrittenSinceLastRotate := uint64(0)

	lastPosition := w.buffered.Position()

	for {
		select {
		case <-w.context.Done():
			return

		case <-flush.Triggered():
			if linesWrittenSinceLastFlush <= 0 {
				continue
			}

			w.logger.Debug("flushing to disk")

			if err := w.writer.Flush(); err != nil {
				w.logger.Error("error while flushing gzip writer", "err", err)

				continue
			}

			if err := w.buffered.Flush(); err != nil {
				w.logger.Error("error while flushing log buffer", "err", err)

				continue
			}

			linesWrittenSinceLastFlush = 0

		case <-rotate.Triggered():
			if bytesWrittenSinceLastRotate <= 0 {
				w.logger.Debug("skipping log rotation since no new logs have been written since last rotation")
				continue
			}

			expoBackoff := backoff.NewExponentialBackOff()
			expoBackoff.InitialInterval = time.Second
			expoBackoff.Multiplier = 2

			for {
				err := w.closeCurrentFile()
				if err == nil {
					break
				}

				sleep := expoBackoff.NextBackOff()
				w.logger.Error("error while performing rotation", "err", err, "waiting", sleep)
				time.Sleep(sleep)
			}

			linesWrittenSinceLastFlush = 0
			bytesWrittenSinceLastRotate = 0

			expoBackoff.Reset()

			for {
				err := w.openLogFile()
				if err == nil {
					break
				}

				sleep := expoBackoff.NextBackOff()
				w.logger.Error("error while performing rotation", "err", err, "waiting", sleep)
				time.Sleep(sleep)
			}

		case req := <-w.request:
			w.logger.Debug("writing log", "bytes", len(req.data))

			if len(req.data) > 1 && req.data[len(req.data)-1] != '\n' {
				req.data = append(req.data, '\n')
			}

			l, err := w.writer.Write(req.data)
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
			newPosition := w.buffered.Position()
			bytesWrittenSinceLastRotate += newPosition - lastPosition
			lastPosition = newPosition

			if bytesWrittenSinceLastRotate > w.maxFileSizeBeforeRotate {
				rotate.Trigger()
			} else if linesWrittenSinceLastFlush > w.maxLogLinesBeforeFlush {
				flush.Trigger()
			}

			labels := prometheus.Labels{
				"source": w.source,
				"level":  req.level,
			}
			w.metrics.total.With(labels).Inc()
			w.metrics.duration.With(labels).Observe(float64(time.Since(req.insertedAt).Seconds()))
			w.metrics.size.With(prometheus.Labels{"source": w.source}).Set(float64(bytesWrittenSinceLastRotate))

			req.result <- nil
		}
	}
}

func (w *worker) stop() error {
	<-w.stopped

	if err := w.writer.Flush(); err != nil {
		return fmt.Errorf("error while flushing gzip writer during shutdown: %w", err)
	}

	if err := w.buffered.Flush(); err != nil {
		return fmt.Errorf("error while flushing log buffer during shutdown: %w", err)
	}

	if err := w.file.Close(); err != nil {
		return fmt.Errorf("error while closing log file during shutdown: %w", err)
	}

	return nil
}
