package desec

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/teapotovh/teapot/lib/observability"
)

var ErrMismatchedDomain = errors.New("mismatched domain name")

func (d *Desec) canConnect(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.client.BaseURL, nil)
	if err != nil {
		return fmt.Errorf("error while building request to base URL: %w", err)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("error while performing GET to base URL: %w", err)
	}

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code on base URL: %d", res.StatusCode)
	}

	return nil
}

func (d *Desec) hasDomain(ctx context.Context) error {
	domain, err := d.client.Domains.Get(ctx, d.domain)
	if err != nil {
		return fmt.Errorf("error while fetching deSEC domain: %w", err)
	}

	if domain.Name != d.domain {
		return fmt.Errorf("%w: expected %s, got %s", ErrMismatchedDomain, d.domain, domain.Name)
	}

	return nil
}

// ReadinessChecks implements observability.ReadinessChecks.
func (d *Desec) ReadinessChecks() map[string]observability.Check {
	return map[string]observability.Check{
		"desec/connect": observability.CheckFunc(d.canConnect),
		"desec/domain":  observability.CheckFunc(d.hasDomain),
	}
}
