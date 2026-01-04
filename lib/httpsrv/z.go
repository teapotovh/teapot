package httpsrv

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/teapotovh/teapot/lib/observability"
)

var ErrNotStartedYet = errors.New("not started yet")

func (h *HTTPSrv) hasServerStarted(ctx context.Context) error {
	if !h.running.Load() {
		return ErrNotStartedYet
	}

	return nil
}

// ReadinessChecks implements observability.ReadinessChecks.
func (h *HTTPSrv) ReadinessChecks() map[string]observability.Check {
	return map[string]observability.Check{
		"httpsrv/started": observability.CheckFunc(h.hasServerStarted),
	}
}

func (h *HTTPSrv) isServerAnswering(ctx context.Context) (err error) {
	url := fmt.Sprintf("http://%s/", h.inner.Addr)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("error while building liveness HTTP request: %w", err)
	}

	req = req.WithContext(ctx)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("error while checking liveness with a GET %s: %w", url, err)
	}

	defer func() {
		if e := res.Body.Close(); e != nil {
			err = fmt.Errorf("error while closing the body of the liveness check request: %w", err)
		}
	}()

	// TODO: does it make sense to verify that the code is not 5xx?
	return nil
}

// LivenessChecks implements observability.LivenessChecks.
func (h *HTTPSrv) LivenessChecks() map[string]observability.Check {
	return map[string]observability.Check{
		"httpsrv/running": observability.CheckFunc(h.isServerAnswering),
	}
}
