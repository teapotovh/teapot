package run

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeService is a Runnable used to exercise the Run lifecycle. It can delay
// its readiness notification, fail after a delay, and spend a configurable
// time shutting down after context cancellation.
type fakeService struct {
	notifyDelay time.Duration
	errAfter    time.Duration // if > 0, return err after this delay post-notify
	runDelay    time.Duration // time spent shutting down after ctx cancel
	err         error
	notified    atomic.Bool
	done        chan struct{}
}

func newFakeService(notifyDelay, runDelay time.Duration, err error) *fakeService {
	return &fakeService{
		notifyDelay: notifyDelay,
		runDelay:    runDelay,
		err:         err,
		done:        make(chan struct{}),
	}
}

func newStartupErrorService(err error) *fakeService {
	return &fakeService{
		errAfter: 50 * time.Millisecond,
		err:      err,
		done:     make(chan struct{}),
	}
}

func (f *fakeService) Run(step Step) error {
	ctx := step.Context()

	if f.notifyDelay > 0 {
		time.Sleep(f.notifyDelay)
	}

	f.notified.Store(true)
	step.Notify()

	if f.errAfter > 0 {
		time.Sleep(f.errAfter)
		close(f.done)

		return f.err
	}

	<-ctx.Done()

	if f.runDelay > 0 {
		time.Sleep(f.runDelay)
	}

	close(f.done)

	return f.err
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(discardWriter{}, nil))
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

func newTestRun() *Run {
	return NewRun(RunConfig{Timeout: 2 * time.Second}, testLogger())
}

func TestStartupSuccess(t *testing.T) {
	t.Parallel()

	r := newTestRun()
	svc := newFakeService(0, 0, nil)
	r.Add("svc", svc, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := r.Run(ctx)
	require.NoError(t, err)
	assert.True(t, svc.notified.Load())
}

func TestNotifyIsIdempotent(t *testing.T) {
	t.Parallel()

	s := &step{ch: make(chan error, 1)}

	var wg sync.WaitGroup

	for range 10 {
		wg.Go(s.Notify)
	}

	wg.Wait()

	require.NoError(t, <-s.ch)

	select {
	case err := <-s.ch:
		t.Fatalf("got a second notification: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestStartupTimeout(t *testing.T) {
	t.Parallel()

	r := NewRun(RunConfig{Timeout: 50 * time.Millisecond}, testLogger())
	r.Add("svc", neverNotifying{}, nil)

	err := r.Run(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTimeout)
}

type neverNotifying struct{}

func (neverNotifying) Run(step Step) error {
	<-step.Context().Done()
	return nil
}

func TestStartupError(t *testing.T) {
	t.Parallel()

	r := newTestRun()
	wantErr := assert.AnError
	r.Add("svc", newStartupErrorService(wantErr), nil)

	err := r.Run(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, wantErr)
}

func TestRuntimeError(t *testing.T) {
	t.Parallel()

	wantErr := assert.AnError
	svc := &runtimeErrorService{err: wantErr, delay: 50 * time.Millisecond}
	r := newTestRun()
	r.Add("svc", svc, nil)

	err := r.Run(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, wantErr)
}

type runtimeErrorService struct {
	err   error
	delay time.Duration
}

func (s *runtimeErrorService) Run(step Step) error {
	step.Notify()

	select {
	case <-step.Context().Done():
		return nil
	case <-time.After(s.delay):
		return s.err
	}
}

// TestRunWaitsForGracefulShutdown ensures that Run does not return before
// every service has completed its graceful shutdown after the context is
// canceled. Regression test for teapotovh/teapot#118: binaries exited the
// process before components finished flushing and closing resources.
func TestRunWaitsForGracefulShutdown(t *testing.T) {
	t.Parallel()

	svc := newFakeService(0, 200*time.Millisecond, nil)
	r := newTestRun()
	r.Add("slow", svc, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	err := r.Run(ctx)
	require.NoError(t, err)

	select {
	case <-svc.done:
		// graceful shutdown completed before Run returned
	default:
		t.Fatalf("Run returned before the service finished shutting down (elapsed %s)", time.Since(start))
	}
}

func TestPanicRecovered(t *testing.T) {
	t.Parallel()

	r := newTestRun()
	r.Add("panicky", panickyService{}, nil)

	err := r.Run(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPanic)
}

type panickyService struct{}

func (panickyService) Run(step Step) error {
	step.Notify()
	panic("boom")
}

// parentService is a Runnable that starts the given children with Wait.
type parentService struct {
	children []Runnable
}

func (p parentService) Run(step Step) error {
	for i, child := range p.children {
		step.Add(string(rune('a'+i)), child)
	}

	step.Wait()
	step.Notify()

	<-step.Context().Done()

	return nil
}

// orderedService sends its name when it starts, and waits for the release
// channel before it notifies.
type orderedService struct {
	name    string
	started chan<- string
	release <-chan struct{}
}

func (s orderedService) Run(step Step) error {
	s.started <- s.name

	<-s.release
	step.Notify()

	<-step.Context().Done()

	return nil
}

// readyService notifies immediately when it starts.
type readyService struct {
	ready chan<- struct{}
}

func (s readyService) Run(step Step) error {
	select {
	case s.ready <- struct{}{}:
	default:
	}

	step.Notify()

	<-step.Context().Done()

	return nil
}

// TestNestedStartsSequentially ensures that Wait starts each child only after
// the previous one notified, and that the parent notifies after all children.
func TestNestedStartsSequentially(t *testing.T) {
	t.Parallel()

	started := make(chan string, 2)
	releaseA := make(chan struct{})
	releaseB := make(chan struct{})
	ready := make(chan struct{}, 1)

	parent := parentService{children: []Runnable{
		orderedService{name: "a", started: started, release: releaseA},
		orderedService{name: "b", started: started, release: releaseB},
	}}

	r := newTestRun()
	r.Add("parent", parent, nil)
	r.Add("ready", readyService{ready: ready}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()

	assert.Equal(t, "a", <-started)

	select {
	case name := <-started:
		t.Fatalf("child %q started before the previous child notified", name)
	case <-time.After(50 * time.Millisecond):
	}

	close(releaseA)
	assert.Equal(t, "b", <-started)

	select {
	case <-ready:
		t.Fatal("parent notified before all children notified")
	case <-time.After(50 * time.Millisecond):
	}

	close(releaseB)
	<-ready

	cancel()
	require.NoError(t, <-done)
}

// TestNestedName ensures that a child component gets the full name of its
// parent, joined with a slash.
func TestNestedName(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	logger := slog.New(slog.NewTextHandler(&buf, nil))

	parent := parentService{children: []Runnable{newFakeService(0, 0, nil)}}

	r := NewRun(RunConfig{Timeout: 2 * time.Second}, logger)
	r.Add("parent", parent, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	require.NoError(t, r.Run(ctx))
	assert.Contains(t, buf.String(), "name=parent/a")
}

// TestNestedStartupError ensures that a failing child stops the parent startup
// with the same error.
func TestNestedStartupError(t *testing.T) {
	t.Parallel()

	wantErr := assert.AnError
	parent := parentService{children: []Runnable{failingService{err: wantErr}}}

	r := newTestRun()
	r.Add("parent", parent, nil)

	err := r.Run(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, wantErr)
}

type failingService struct{ err error }

func (s failingService) Run(Step) error {
	return s.err
}

// TestNestedWaitTimeout ensures that a child that never notifies makes the
// parent startup fail.
func TestNestedWaitTimeout(t *testing.T) {
	t.Parallel()

	parent := parentService{children: []Runnable{neverNotifying{}}}

	r := NewRun(RunConfig{Timeout: 50 * time.Millisecond}, testLogger())
	r.Add("parent", parent, nil)

	err := r.Run(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTimeout))
}

// cancelWatcher notifies when it starts, then closes done when its context is
// canceled.
type cancelWatcher struct {
	done chan struct{}
}

func (s cancelWatcher) Run(step Step) error {
	step.Notify()

	<-step.Context().Done()
	close(s.done)

	return nil
}

// TestRuntimeErrorCancelsOtherServices ensures that a runtime error in one
// service cancels the context of the other services.
func TestRuntimeErrorCancelsOtherServices(t *testing.T) {
	t.Parallel()

	wantErr := assert.AnError
	watcher := cancelWatcher{done: make(chan struct{})}

	r := newTestRun()
	r.Add("failing", &runtimeErrorService{err: wantErr, delay: 20 * time.Millisecond}, nil)
	r.Add("watcher", watcher, nil)

	err := r.Run(context.Background())
	require.ErrorIs(t, err, wantErr)

	select {
	case <-watcher.done:
	case <-time.After(time.Second):
		t.Fatal("the failing service did not cancel the other service")
	}
}

// TestNestedRuntimeError ensures that a runtime error in a sub-component stops
// the whole run and cancels its siblings.
func TestNestedRuntimeError(t *testing.T) {
	t.Parallel()

	wantErr := assert.AnError
	watcher := cancelWatcher{done: make(chan struct{})}

	parent := parentService{children: []Runnable{
		&runtimeErrorService{err: wantErr, delay: 20 * time.Millisecond},
		watcher,
	}}

	r := newTestRun()
	r.Add("parent", parent, nil)

	err := r.Run(context.Background())
	require.ErrorIs(t, err, wantErr)

	select {
	case <-watcher.done:
	case <-time.After(time.Second):
		t.Fatal("the failing sub-component did not cancel its sibling")
	}
}

// TestNestedPanic ensures that a panic in a sub-component becomes an error.
func TestNestedPanic(t *testing.T) {
	t.Parallel()

	parent := parentService{children: []Runnable{panickyService{}}}

	r := newTestRun()
	r.Add("parent", parent, nil)

	err := r.Run(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPanic)
}

// TestNestedGracefulShutdown ensures that Run waits for nested components to
// finish their shutdown after the context is canceled.
func TestNestedGracefulShutdown(t *testing.T) {
	t.Parallel()

	svc := newFakeService(0, 100*time.Millisecond, nil)
	parent := parentService{children: []Runnable{svc}}
	ready := make(chan struct{}, 1)

	r := newTestRun()
	r.Add("parent", parent, nil)
	r.Add("ready", readyService{ready: ready}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() { done <- r.Run(ctx) }()

	<-ready
	cancel()

	require.NoError(t, <-done)

	select {
	case <-svc.done:
	default:
		t.Fatal("Run returned before the nested component finished shutting down")
	}
}

// TestNotifyErrorIsFinal ensures that no notification is sent after an error.
func TestNotifyErrorIsFinal(t *testing.T) {
	t.Parallel()

	s := &step{ch: make(chan error, 1)}
	s.notifyError(assert.AnError)
	s.Notify()

	require.ErrorIs(t, <-s.ch, assert.AnError)

	select {
	case err := <-s.ch:
		t.Fatalf("got a notification after an error: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
}
