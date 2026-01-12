package alert

import (
	"context"
	"fmt"

	"github.com/teapotovh/teapot/lib/observability"
)

func (a *Alert) canConnect(ctx context.Context) error {
	_, _, err := a.client.Users.Get(ctx, "")
	if err != nil {
		return fmt.Errorf("error while check connectivity to github by performing an empty user get: %w", err)
	}

	return nil
}

func (a *Alert) repoExists(ctx context.Context) error {
	_, _, err := a.client.Repositories.Get(ctx, a.owner, a.repo)
	if err != nil {
		return fmt.Errorf("error while checking if the repository \"%s/%s\" exists: %w", a.owner, a.repo, err)
	}

	return nil
}

// ReadinessChecks implements observability.ReadinessChecks.
func (a *Alert) ReadinessChecks() map[string]observability.Check {
	return map[string]observability.Check{
		"alert/connect": observability.CheckFunc(a.canConnect),
		"alert/repo":    observability.CheckFunc(a.repoExists),
	}
}
