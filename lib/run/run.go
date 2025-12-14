package run

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"
)

var ErrOtherService = errors.New("terminating as other service failed")

type Notify interface {
	Notify()
}

type notify struct {
	ch   chan error
	sent bool
}

// Notify implements Notify.
func (n *notify) Notify() {
	n.sent = true
	// don't wait for the receiving end to receive the notify
	select {
	case n.ch <- nil:
	default:
	}
}

// this function is only used internally to signal an immediate crash
// before the timeout period.
func (n *notify) notifyError(err error) {
	n.ch <- err
}

type Runnable interface {
	Run(ctx context.Context, notify Notify) error
}

type runnable struct {
	runnable Runnable
	logger   *slog.Logger
	errors   chan error
	name     string
	timeout  time.Duration
}

func (r runnable) startup(ctx context.Context) error {
	notify := &notify{ch: make(chan error, 1)}

	go r.watch(ctx, notify)

	start := time.Now()

	tick := time.Tick(r.timeout)
	select {
	case <-tick:
		return fmt.Errorf("service %q startup timed out after %s", r.name, r.timeout)
	case err := <-notify.ch:
		if err != nil {
			return err
		} else {
			// startup was successful, as we received the notification
			r.logger.Info("successfully started component", "elapsed", time.Since(start))
			return nil
		}
	}
}

func (r runnable) watch(ctx context.Context, ntfy *notify) {
	err := r.wrap(ctx, ntfy)
	if err != nil {
		// the outer service is still waiting for timeout (since no notification
		// was fired), thus send a notification error.
		if !ntfy.sent {
			ntfy.notifyError(err)
		} else {
			r.errors <- err
		}
	}
}

// wrap wraps the Runnable service with panic-handling code, so that we return
// a normal error even if the runnable panics.
func (r runnable) wrap(ctx context.Context, ntfy *notify) (err error) {
	defer func() {
		if recover() != nil {
			trace := string(debug.Stack())
			err = fmt.Errorf("component %q panicked: %s", r.name, trace)
		}
	}()

	err = r.runnable.Run(ctx, ntfy)

	return
}

type Run struct {
	logger   *slog.Logger
	errors   chan error
	services []runnable
	timeout  time.Duration
}

type RunConfig struct {
	Timeout time.Duration
}

func NewRun(config RunConfig, logger *slog.Logger) *Run {
	return &Run{
		logger:  logger,
		timeout: config.Timeout,
		errors:  make(chan error),
	}
}

func (r *Run) Add(name string, run Runnable, timeout *time.Duration) {
	t := r.timeout
	if timeout != nil {
		t = *timeout
	}

	r.services = append(r.services, runnable{
		logger:   r.logger.With("name", name),
		name:     name,
		runnable: run,
		timeout:  t,
		errors:   r.errors,
	})
}

func (r *Run) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(ErrOtherService)

	for _, svc := range r.services {
		r.logger.Debug("starting service", "name", svc.name, "timeout", svc.timeout)
		// This runs the service in the background, unless there is a failure at
		// startup, in which case it returns immediately (or after the specified
		// timeout) with an error.
		//
		// If the service startup succeeds, runtime errors can be monitored via the
		// `errors` channel.
		if err := svc.startup(ctx); err != nil {
			return fmt.Errorf("error while starting service: %w", err)
		}
	}

	done := ctx.Done()

	var err error
	select {
	case <-done:
		break
	case err = <-r.errors:
		break
	}

	return err
}
