package log

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/teapotovh/teapot/lib/run"
)

var (
	ErrTerminating = errors.New("process terminating")

	DirFileMode = os.FileMode(0o0750)
)

type WorkerManager struct {
	logger  *slog.Logger
	context context.Context

	directory               string
	flushInterval           time.Duration
	maxLogLinesBeforeFlush  uint32
	rotateInterval          time.Duration
	maxFileSizeBeforeRotate uint64
	capacity                uint32

	terminating atomic.Bool
	workers     sync.Map
	metrics     *metrics
}

func NewWorkerManager(
	path string,
	flushInterval time.Duration,
	maxLogLinesBeforeFlush uint32,
	rotateInterval time.Duration,
	maxFileSizeBeforeRotate uint64,
	capacity uint32,
	metrics *metrics,
	logger *slog.Logger,
) *WorkerManager {
	return &WorkerManager{
		logger: logger,

		directory:               path,
		flushInterval:           flushInterval,
		maxLogLinesBeforeFlush:  maxLogLinesBeforeFlush,
		rotateInterval:          rotateInterval,
		maxFileSizeBeforeRotate: maxFileSizeBeforeRotate,
		capacity:                capacity,
		metrics:                 metrics,
	}
}

// Run implements run.Runnable.
func (m *WorkerManager) Run(ctx context.Context, notify run.Notify) (err error) {
	m.context = ctx

	notify.Notify()

	<-ctx.Done()
	m.terminating.Store(true)
	m.workers.Range(func(key, value any) bool {
		worker := value.(*worker)

		err := worker.stop()
		if err != nil {
			m.logger.Error("error while stopping worker", "source", key, "err", err)
		}

		return true
	})

	return nil
}

func (m *WorkerManager) process(e event, level string) error {
	if m.terminating.Load() {
		return ErrTerminating
	}

	w, err := m.worker(e.Source)
	if err != nil {
		return fmt.Errorf("could not get worker: %w", err)
	}

	result := make(chan error)
	w.request <- workerRequest{
		data:       e.Data,
		result:     result,
		insertedAt: time.Now(),
		level:      level,
	}

	return <-result
}

func (m *WorkerManager) logPath(source string) string {
	return filepath.Join(m.directory, source)
}

func (m *WorkerManager) worker(source string) (*worker, error) {
	w, ok := m.workers.Load(source)
	if !ok {
		p := m.logPath(source)
		if err := os.MkdirAll(p, DirFileMode); err != nil {
			return nil, fmt.Errorf("could not create log directory %q: %w", p, err)
		}

		l := m.logger.With("source", source, "component", "worker")

		nw, err := newWorker(
			m.context,
			source,
			p,
			m.flushInterval,
			m.maxLogLinesBeforeFlush,
			m.rotateInterval,
			m.maxFileSizeBeforeRotate,
			m.capacity,
			m.metrics,
			l,
		)
		if err != nil {
			return nil, fmt.Errorf("error while creating worker for source %q: %w", source, err)
		}

		go nw.run()

		w = nw
		m.workers.Store(source, w)
	}

	return w.(*worker), nil
}
