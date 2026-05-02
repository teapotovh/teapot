package log

import (
	"context"
	"encoding/json"
	"log/slog"
)

type unit struct{}

type worker struct {
	logger  *slog.Logger
	context context.Context

	request chan workerRequest
	stopped chan unit
}

type workerRequest struct {
	data   json.RawMessage
	result chan error
}

func newWorker(ctx context.Context, capacity uint32, logger *slog.Logger) *worker {
	return &worker{
		logger:  logger,
		context: ctx,

		request: make(chan workerRequest, capacity),
		stopped: make(chan unit, 1),
	}
}

func (w *worker) run() {
	for {
		select {
		case <-w.context.Done():
			return

		case req := <-w.request:
			w.logger.Info("got request", "request", req)
			req.result <- nil
		}
	}
}

func (w *worker) stop() {
	<-w.stopped
}
