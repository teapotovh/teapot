package run

import (
	"context"
	"log/slog"
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

func (f *fakeService) Run(ctx context.Context, notify Notify) error {
	if f.notifyDelay > 0 {
		time.Sleep(f.notifyDelay)
	}

	f.notified.Store(true)
	notify.Notify()

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

func TestStartupTimeout(t *testing.T) {
	t.Parallel()

	r := NewRun(RunConfig{Timeout: 50 * time.Millisecond}, testLogger())
	r.Add("svc", neverNotifying{}, nil)

	err := r.Run(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTimeout)
}

type neverNotifying struct{}

func (neverNotifying) Run(ctx context.Context, notify Notify) error {
	<-ctx.Done()
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

func (s *runtimeErrorService) Run(ctx context.Context, notify Notify) error {
	notify.Notify()

	select {
	case <-ctx.Done():
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

func (panickyService) Run(ctx context.Context, notify Notify) error {
	notify.Notify()
	panic("boom")
}
