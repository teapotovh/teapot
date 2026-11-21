// Package run starts and monitors long-running components.
//
// Add each component with [Run.Add]. [Run.Run] starts the components in order
// and waits for each one to notify that it started. If a component fails to
// start, or the startup timeout expires, Run.Run stops and returns the error.
//
// After all components start, Run.Run waits until the context is canceled or a
// component fails. If one component fails, Run.Run cancels the context of all
// components and returns the error. A panic in a component becomes an error
// too.
//
// Run calls [Runnable.Run] at most once. If Runnable.Run returns an error, the
// whole process stops. When the context passed to Runnable.Run is canceled, the
// process will stop soon.
//
// Do not add defensive code for many calls to Runnable.Run. Use these
// properties to make Runnable.Run as simple as possible.
package run

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"
)

var (
	ErrOtherService = errors.New("terminating as other service failed")
	ErrPanic        = errors.New("caught panic")
	ErrTimeout      = errors.New("timed out")
)

type Notify interface {
	Notify()
}

type notify struct {
	ch   chan error
	sent atomic.Bool
}

// Notify implements Notify.
func (n *notify) Notify() {
	if !n.sent.CompareAndSwap(false, true) {
		// If the compare failed, the value was not false (we had already sent the
		// notify). Let's be idempotent and just do nothing here.
		return
	}

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

// Runnable is a long-running component.
//
// Run is called at most once. A returned error stops the whole process. A
// canceled context means that the process will stop soon, so Run must return.
type Runnable interface {
	Run(ctx context.Context, notify Notify) error
}

type runnable struct {
	runnable Runnable
	logger   *slog.Logger
	name     string
	timeout  time.Duration
}

func (r runnable) startup(ctx context.Context, g *errgroup.Group) error {
	notify := &notify{ch: make(chan error, 1)}

	g.Go(func() error {
		return r.watch(ctx, notify)
	})

	start := time.Now()

	tick := time.Tick(r.timeout)
	select {
	case <-tick:
		return fmt.Errorf("service %q failed to start after %s: %w", r.name, r.timeout, ErrTimeout)
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

func (r runnable) watch(ctx context.Context, ntfy *notify) error {
	err := r.wrap(ctx, ntfy)
	if err != nil {
		// the outer service is still waiting for timeout (since no notification
		// was fired), thus send a notification error.
		if !ntfy.sent.Load() {
			ntfy.notifyError(err)
		}
	}

	return err
}

// wrap wraps the Runnable service with panic-handling code, so that we return
// a normal error even if the runnable panics.
func (r runnable) wrap(ctx context.Context, ntfy *notify) (err error) {
	defer func() {
		if recover() != nil {
			trace := string(debug.Stack())
			err = fmt.Errorf("error in component %q: %w. Trace: %s", r.name, ErrPanic, trace)
		}
	}()

	err = r.runnable.Run(ctx, ntfy)

	return
}

type Run struct {
	logger   *slog.Logger
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
	})
}

func (r *Run) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(ErrOtherService)

	g, ctx := errgroup.WithContext(ctx)

	for _, svc := range r.services {
		r.logger.Debug("starting service", "name", svc.name, "timeout", svc.timeout)
		// This runs the service in the background, unless there is a failure at
		// startup, in which case it returns immediately (or after the specified
		// timeout) with an error.
		//
		// If the service startup succeeds, runtime errors cancel the context and
		// are returned by the group.
		if err := svc.startup(ctx, g); err != nil {
			return fmt.Errorf("error while starting service: %w", err)
		}
	}

	return g.Wait()
}
