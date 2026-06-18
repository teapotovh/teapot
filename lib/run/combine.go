package run

import (
	"context"
	"fmt"
	"sync/atomic"

	"golang.org/x/sync/errgroup"
)

type combinedNotify struct {
	lower   Notify
	total   uint64
	counter atomic.Uint64
}

// Notify implements Notify.
func (n *combinedNotify) Notify() {
	n.counter.Add(1)

	if n.counter.Load() == n.total {
		n.lower.Notify()
	}
}

type CombinedRun struct {
	runnables []Runnable
}

func Combine(runners ...Runnable) *CombinedRun {
	return &CombinedRun{runnables: runners}
}

// Run implements Runnable
func (c *CombinedRun) Run(ctx context.Context, notify Notify) error {
	cn := &combinedNotify{lower: notify, total: uint64(len(c.runnables))}

	eg := errgroup.Group{}
	errCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	for _, runnable := range c.runnables {
		eg.Go(func() error {
			if err := runnable.Run(errCtx, cn); err != nil {
				cancel(fmt.Errorf("other Runnable failed with: %w", err))
				return err
			}

			return nil
		})
	}

	return eg.Wait()
}

// Ensure *CombinedRun implements Run
var _ Runnable = &CombinedRun{}
