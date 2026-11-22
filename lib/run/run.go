// Package run starts and monitors long-running components.
//
// Add each component with [Run.Add]. [Run.Run] starts the components in order
// and waits for each one to notify that it started. A component can add
// sub-components with [Step.Add] and start them with [Step.Wait]. If a
// component fails to start, or the startup timeout expires, Run.Run stops and
// returns the error.
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
	"path"
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

// Step is the startup context of a Runnable.
//
// A Runnable must call Notify after it starts. It can add sub-components with
// Add and start them with Wait. Add must be called before Wait. A Step is not
// safe for concurrent use.
type Step interface {
	// Context returns the context of the Runnable. The context is canceled when
	// the process stops.
	Context() context.Context
	// Notify reports that the Runnable started.
	Notify()
	// Add adds a sub-component. The full name of the sub-component is the name
	// of the parent, followed by "/" and the given name. Wait starts the
	// sub-components in order and waits for each one to notify.
	Add(name string, runnable Runnable)
	// Wait starts all the sub-components added with Add and waits for them to
	// notify. If a sub-component fails to start, Wait reports the error to the
	// parent and stops starting the remaining sub-components.
	Wait()
}

// Runnable is a long-running component.
//
// Run is called at most once. A returned error stops the whole process. A
// canceled context means that the process will stop soon, so Run must return.
type Runnable interface {
	Run(step Step) error
}

type step struct {
	logger   *slog.Logger
	children []runnable
	timeout  time.Duration

	ctx      context.Context
	ch       chan error
	notified atomic.Bool
	name     string
	group    *errgroup.Group
}

// Context implements Step.
func (s *step) Context() context.Context {
	return s.ctx
}

// Notify implements Step. It is safe to call multiple times.
func (s *step) Notify() {
	if !s.notified.CompareAndSwap(false, true) {
		// We already sent the notification, so do nothing. This keeps Notify
		// idempotent.
		return
	}

	// don't wait for the receiving end to receive the notify
	select {
	case s.ch <- nil:
	default:
	}
}

// Add implements Step.
func (s *step) Add(name string, component Runnable) {
	name = path.Join(s.name, name)

	s.children = append(s.children, runnable{
		logger:   s.logger,
		name:     name,
		runnable: component,
		timeout:  s.timeout,
	})
}

// Wait implements Step.
func (s *step) Wait() {
	children := s.children
	s.children = nil

	if err := start(s.ctx, s.group, children); err != nil {
		s.notifyError(err)
	}
}

// notifyError signals an immediate crash before the timeout period. It does
// nothing if the Runnable already notified.
func (s *step) notifyError(err error) {
	if !s.notified.CompareAndSwap(false, true) {
		return
	}

	s.ch <- err
}

type runnable struct {
	runnable Runnable
	logger   *slog.Logger
	name     string
	timeout  time.Duration
}

func (r runnable) startup(ctx context.Context, group *errgroup.Group) error {
	r.logger.Debug("starting component", "name", r.name, "timeout", r.timeout)

	stepper := &step{
		ctx:     ctx,
		ch:      make(chan error, 1),
		logger:  r.logger,
		name:    r.name,
		timeout: r.timeout,
		group:   group,
	}

	group.Go(func() error {
		return r.watch(stepper)
	})

	start := time.Now()

	timer := time.NewTimer(r.timeout)
	defer timer.Stop()

	select {
	case <-timer.C:
		return fmt.Errorf("service %q failed to start after %s: %w", r.name, r.timeout, ErrTimeout)
	case err := <-stepper.ch:
		if err != nil {
			return err
		}

		// startup was successful, as we received the notification
		r.logger.Info("successfully started component", "name", r.name, "elapsed", time.Since(start))

		return nil
	}
}

func (r runnable) watch(stepper *step) error {
	err := r.wrap(stepper)
	if err != nil {
		// The outer component may still be waiting for this component to notify.
		// Send the error to stop the startup wait early.
		stepper.notifyError(err)
	}

	return err
}

// wrap wraps the Runnable component with panic-handling code, so that we
// return a normal error even if the runnable panics.
func (r runnable) wrap(stepper *step) (err error) {
	defer func() {
		if recover() != nil {
			trace := string(debug.Stack())
			err = fmt.Errorf("error in component %q: %w. Trace: %s", r.name, ErrPanic, trace)
		}
	}()

	err = r.runnable.Run(stepper)

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
		logger:   r.logger,
		name:     name,
		runnable: run,
		timeout:  t,
	})
}

// start starts the components in order and stops at the first startup error.
// Every component uses the same group, so a runtime failure anywhere cancels the
// context of the whole tree and is returned by the group in Run.Run.
func start(ctx context.Context, group *errgroup.Group, components []runnable) error {
	for _, component := range components {
		if err := component.startup(ctx, group); err != nil {
			return err
		}
	}

	return nil
}

func (r *Run) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(ErrOtherService)

	group, ctx := errgroup.WithContext(ctx)

	// This starts each service in the background, unless there is a failure at
	// startup, in which case it returns immediately (or after the specified
	// timeout) with an error. If a service startup succeeds, runtime errors
	// cancel the context and are returned by the group.
	if err := start(ctx, group, r.services); err != nil {
		return fmt.Errorf("error while starting service: %w", err)
	}

	return group.Wait()
}
