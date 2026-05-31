package bottin

import (
	"context"
	"fmt"

	"github.com/teapotovh/teapot/lib/observability"
)

func (server *Bottin) canPingStore(ctx context.Context) error {
	err := server.store.Ping(ctx)
	if err != nil {
		return fmt.Errorf("could not ping the store: %w", err)
	}

	return nil
}

// ReadinessChecks implements observability.ReadinessChecks.
func (server *Bottin) ReadinessChecks() map[string]observability.Check {
	return map[string]observability.Check{
		"bottin/store/ping": observability.CheckFunc(server.canPingStore),
	}
}
