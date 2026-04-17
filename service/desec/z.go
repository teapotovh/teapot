package desec

import (
	"context"

	"github.com/teapotovh/teapot/lib/observability"
)

func (d *Desec) canConnect(ctx context.Context) error {
	// TODO: implement when we have the API client
	return nil
}

func (d *Desec) canAuthenticate(ctx context.Context) error {
	// TODO: implement when we have the API client
	return nil
}

func (d *Desec) hasDomain(ctx context.Context) error {
	// TODO: implement when we have the API client
	return nil
}

// ReadinessChecks implements observability.ReadinessChecks.
func (d *Desec) ReadinessChecks() map[string]observability.Check {
	return map[string]observability.Check{
		"desec/connect": observability.CheckFunc(d.canConnect),
		"desec/auth":    observability.CheckFunc(d.canAuthenticate),
		"desec/domain":  observability.CheckFunc(d.hasDomain),
	}
}
