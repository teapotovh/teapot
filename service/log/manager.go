package log

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/teapotovh/teapot/lib/run"
)

var ErrTerminating = errors.New("process terminating")

type Manager struct {
	logger  *slog.Logger
	context context.Context

	capacity    uint32
	terminating atomic.Bool
	workers     sync.Map
}

func NewManager(capacity uint32, logger *slog.Logger) *Manager {
	return &Manager{
		logger: logger,
	}
}

// Run implements run.Runnable.
func (m *Manager) Run(ctx context.Context, notify run.Notify) (err error) {
	m.context = ctx
	notify.Notify()

	<-ctx.Done()
	m.terminating.Store(true)
	m.workers.Range(func(key, value any) bool {
		worker := value.(*worker)
		worker.stop()
		return true
	})

	return nil
}

func (m *Manager) process(e event) error {
	if m.terminating.Load() {
		return ErrTerminating
	}

	w := m.worker(e.Source)
	result := make(chan error)
	w.request <- workerRequest{
		data:   e.Data,
		result: result,
	}

	return <-result
}

func (m *Manager) worker(source string) *worker {
	w, ok := m.workers.Load(source)
	if !ok {
		nw := newWorker(m.context, m.capacity, m.logger.With("source", source))
		go nw.run()

		w = nw
		m.workers.Store(source, w)
	}

	return w.(*worker)
}
