package externalip

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/netip"
	"time"

	"github.com/cenkalti/backoff/v5"

	"github.com/teapotovh/teapot/lib/run"
	"github.com/teapotovh/teapot/service/ccm"
)

type ExternalIPConfig struct {
	Server     string
	RetryDelay time.Duration
	MaxRetries uint64
	Interval   time.Duration
}

type ExternalIP struct {
	externalIP netip.Addr
	logger     *slog.Logger
	ccm        *ccm.CCM
	httpClient http.Client
	server     string
	retryDelay time.Duration
	maxRetries uint64
	interval   time.Duration
}

func NewExternalIP(ccm *ccm.CCM, config ExternalIPConfig, logger *slog.Logger) (*ExternalIP, error) {
	return &ExternalIP{
		logger: logger,
		ccm:    ccm,

		server:     config.Server,
		retryDelay: config.RetryDelay,
		maxRetries: config.MaxRetries,
		interval:   config.Interval,
	}, nil
}

func (eip *ExternalIP) fetchPublicIP(ctx context.Context) (netip.Addr, error) {
	f := func() (addr netip.Addr, err error) {
		defer func() {
			if err != nil {
				eip.logger.Warn("failed to fetch public IP address", "error", err)
			}
		}()

		req, err := http.NewRequest(http.MethodGet, eip.server, nil)
		if err != nil {
			return addr, fmt.Errorf("error creating request: %w", err)
		}

		resp, err := eip.httpClient.Do(req.WithContext(ctx))
		if err != nil {
			return addr, fmt.Errorf("error performing request: %w", err)
		}

		defer func() {
			if e := resp.Body.Close(); e != nil {
				err = fmt.Errorf("error while closing request body: %w", e)
			}
		}()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return addr, fmt.Errorf("error reading response body: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return addr, fmt.Errorf("unexpected response status: %d, body: %s", resp.StatusCode, string(body))
		}

		addr, err = netip.ParseAddr(string(body))
		if err != nil {
			return addr, fmt.Errorf("error parsing IP address from response: %w", err)
		}

		return addr, nil
	}

	expoBackoff := backoff.NewExponentialBackOff()
	expoBackoff.InitialInterval = eip.retryDelay
	expoBackoff.Multiplier = 2

	return backoff.Retry(ctx, f, backoff.WithMaxTries(uint(eip.maxRetries)), backoff.WithBackOff(expoBackoff))
}

func (eip *ExternalIP) setExternalIP(ctx context.Context, ip netip.Addr, source string) error {
	if err := eip.ccm.SetExternalIP(ctx, ip); err != nil {
		return fmt.Errorf("error while updating node ExternalIP (source: %s): %w", source, err)
	} else {
		eip.logger.Info("updated external IP", "ip", ip, "old", eip.externalIP, "source", source)
		eip.externalIP = ip

		return nil
	}
}

// Run implements run.Runnable.
func (eip *ExternalIP) Run(ctx context.Context, notify run.Notify) error {
	sub := eip.ccm.Broker().Subscribe()
	defer sub.Unsubscribe()

	notify.Notify()

	ticker := time.NewTicker(eip.interval)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			publicIP, err := eip.fetchPublicIP(ctx)
			if err != nil {
				return fmt.Errorf("error while fetching public IP: %w", err)
			}

			if eip.externalIP == publicIP {
				eip.logger.Debug("public IP has not changed, skipping ExternalIP update", "ip", publicIP)
				continue
			}

			if err := eip.setExternalIP(ctx, publicIP, "ddns"); err != nil {
				return err
			}
		case event := <-sub.Chan():
			eip.logger.Debug("received CCM event", "event", event)

			if eip.externalIP.IsValid() {
				// Ensure noone else tampers with ExternalIP
				if event.ExternalIP != eip.externalIP {
					if err := eip.setExternalIP(ctx, eip.externalIP, "event"); err != nil {
						return err
					}
				}
			} else {
				eip.externalIP = event.ExternalIP
			}
		}
	}
}
