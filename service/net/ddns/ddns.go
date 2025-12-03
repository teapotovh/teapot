package ddns

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
	tnet "github.com/teapotovh/teapot/service/net"
	"github.com/teapotovh/teapot/service/net/internal"
)

type DDNSConfig struct {
	Server     string
	RetryDelay time.Duration
	MaxRetries uint64
	Interval   time.Duration
}

type DDNS struct {
	logger *slog.Logger
	net    *tnet.Net

	node       string
	externalIP netip.Addr

	server     string
	retryDelay time.Duration
	maxRetries uint64
	interval   time.Duration

	httpClient http.Client
}

func NewDDNS(net *tnet.Net, config DDNSConfig, logger *slog.Logger) (*DDNS, error) {
	logger.Debug("config", "config", config)
	return &DDNS{
		logger: logger,
		net:    net,

		server:     config.Server,
		retryDelay: config.RetryDelay,
		maxRetries: config.MaxRetries,
		interval:   config.Interval,
	}, nil
}

func (d *DDNS) fetchPublicIP(ctx context.Context) (netip.Addr, error) {
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

// Run implements run.Runnable
func (d *DDNS) Run(ctx context.Context, notify run.Notify) error {
	lsub := d.net.Local().Broker().Subscribe()
	defer lsub.Unsubscribe()

	notify.Notify()
	ticker := time.NewTicker(d.interval)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if d.node == "" || !d.externalIP.IsValid() {
				d.logger.Warn("skipping DDNS update as local node information is not available yet")
				continue
			}
			publicIP, err := d.fetchPublicIP(ctx)
			if err != nil {
				return fmt.Errorf("error while fetching public IP: %w", err)
			}

			if d.externalIP == publicIP {
				d.logger.Debug("public IP has not changed, skipping DDNS update", "ip", publicIP)
				continue
			}
			if err := internal.AnnotateNode(ctx, d.net.Client(), d.node, tnet.AnnotationExternalIP, publicIP.String()); err != nil {
				return fmt.Errorf("error while storing external IP in node %q annotation: %w", d.node, err)
			} else {
				d.logger.Info("updated external IP", "node", d.node, "ip", publicIP)
			}
		case local := <-lsub.Chan():
			d.node = local.Node
			d.externalIP = local.Address
		}
	}
}
