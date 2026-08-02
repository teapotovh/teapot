package calendar

import (
	"context"
	"fmt"
	"maps"

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
	calendar := map[string]observability.Check{
		"calendar/ping": observability.CheckFunc(c.canPingStore),
	}
	ldap := c.ldapFactory.LivenessChecks()

	maps.Copy(calendar, ldap)

	return calendar
}

// LivenessChecks implements observability.LivenessChecks.
func (c *Calendar) LivenessChecks() map[string]observability.Check {
	return c.ldapFactory.LivenessChecks()
}
