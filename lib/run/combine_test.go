package run

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingNotify counts how many times the top-level Notify of a CombinedRun
// is called.
type countingNotify struct {
	notify

	count atomic.Int64
}

func newCountingNotify() *countingNotify {
	return &countingNotify{ch: make(chan error, 1)}
}

func (n *countingNotify) Notify() {
	n.count.Add(1)
	n.notify.Notify()
}

// gateService waits on a gate channel before notifying, so the test controls
// when the combined notify counter reaches its total.
type gateService struct {
	gate chan struct{}
}

func (s gateService) Run(ctx context.Context, notify Notify) error {
	<-s.gate
	notify.Notify()
	<-ctx.Done()

	return nil
}

// TestCombinedNotifyRequiresAllRunnables ensures that the lower-level Notify
// fires only after every combined Runnable has notified.
func TestCombinedNotifyRequiresAllRunnables(t *testing.T) {
	t.Parallel()

	gateA := make(chan struct{})
	gateB := make(chan struct{})
	lower := newCountingNotify()
	combined := Combine(gateService{gate: gateA}, gateService{gate: gateB})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- combined.Run(ctx, lower) }()

	// One runnable notified, but not all of them: the lower notify must not
	// have fired yet.
	close(gateA)
	time.Sleep(50 * time.Millisecond)
	assert.EqualValues(t, 0, lower.count.Load())

	// Notify the last runnable: now the lower notify must fire exactly once.
	close(gateB)

	select {
	case err := <-lower.ch:
		_ = err
	case <-time.After(time.Second):
		t.Fatal("lower notify did not fire after all runnables notified")
	}

	assert.EqualValues(t, 1, lower.count.Load())

	cancel()
	require.NoError(t, <-errCh)
}

// TestCombinedCancelPropagates ensures that a failing runnable cancels the
// context passed to the other runnables.
func TestCombinedCancelPropagates(t *testing.T) {
	t.Parallel()

	blocker := blockerService{done: make(chan struct{})}
	combined := Combine(failingService{err: assert.AnError}, blocker)

	err := combined.Run(context.Background(), &notify{ch: make(chan error, 1)})
	require.Error(t, err)
	assert.ErrorIs(t, err, assert.AnError)

	select {
	case <-blocker.done:
		// the blocker observed cancellation and exited
	default:
		t.Fatal("failing runnable did not cancel the other runnables")
	}
}

type failingService struct{ err error }

func (s failingService) Run(ctx context.Context, notify Notify) error {
	notify.Notify()
	return s.err
}

type blockerService struct {
	done chan struct{}
}

func (s blockerService) Run(ctx context.Context, notify Notify) error {
	notify.Notify()
	<-ctx.Done()
	close(s.done)

	return nil
}
