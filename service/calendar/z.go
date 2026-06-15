package calendar

import (
	"context"
	"fmt"

	"github.com/teapotovh/teapot/lib/observability"
)

func (c *Calendar) canPingStore(ctx context.Context) error {
	err := c.store.Ping(ctx)
	if err != nil {
		return fmt.Errorf("could not ping the store: %w", err)
	}

	return nil
}

// ReadinessChecks implements observability.ReadinessChecks.
func (c *Calendar) ReadinessChecks() map[string]observability.Check {
	return map[string]observability.Check{
		"calendar/ping": observability.CheckFunc(c.canPingStore),
	}
}
