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
	logger *slog.Logger
	ccm    *ccm.CCM

	externalIP netip.Addr

	server     string
	retryDelay time.Duration
	maxRetries uint64
	interval   time.Duration

	httpClient http.Client
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

func (d *ExternalIP) fetchPublicIP(ctx context.Context) (netip.Addr, error) {
	f := func() (addr netip.Addr, err error) {
		defer func() {
			if err != nil {
				d.logger.Warn("failed to fetch public IP address", "error", err)
			}
		}()

		req, err := http.NewRequest(http.MethodGet, d.server, nil)
		if err != nil {
			return addr, fmt.Errorf("error creating request: %w", err)
		}

		resp, err := d.httpClient.Do(req.WithContext(ctx))
		if err != nil {
			return addr, fmt.Errorf("error performing request: %w", err)
		}
		defer resp.Body.Close()

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
	expoBackoff.InitialInterval = d.retryDelay
	expoBackoff.Multiplier = 2
	return backoff.Retry(ctx, f, backoff.WithMaxTries(uint(d.maxRetries)), backoff.WithBackOff(expoBackoff))
}

func (d *ExternalIP) setExternalIP(ctx context.Context, ip netip.Addr, source string) error {
	if err := d.ccm.SetExternalIP(ctx, ip); err != nil {
		return fmt.Errorf("error while updating node ExternalIP (source: %s): %w", source, err)
	} else {
		d.logger.Info("updated external IP", "ip", ip, "old", d.externalIP, "source", source)
		d.externalIP = ip
		return nil
	}
}

// Run implements run.Runnable
func (d *ExternalIP) Run(ctx context.Context, notify run.Notify) error {
	sub := d.ccm.Broker().Subscribe()
	defer sub.Unsubscribe()

	notify.Notify()
	ticker := time.NewTicker(d.interval)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			publicIP, err := d.fetchPublicIP(ctx)
			if err != nil {
				return fmt.Errorf("error while fetching public IP: %w", err)
			}

			if d.externalIP == publicIP {
				d.logger.Debug("public IP has not changed, skipping ExternalIP update", "ip", publicIP)
				continue
			}

			if err := d.setExternalIP(ctx, publicIP, "ddns"); err != nil {
				return err
			}
		case event := <-sub.Chan():
			d.logger.Debug("received CCM event", "event", event)

			if d.externalIP.IsValid() {
				// Ensure noone else tampers with ExternalIP
				if event.ExternalIP != d.externalIP {
					if err := d.setExternalIP(ctx, d.externalIP, "event"); err != nil {
						return err
					}
				}
			} else {
				d.externalIP = event.ExternalIP
			}
		}
	}
}
