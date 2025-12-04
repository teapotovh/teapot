package internalip

import (
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"

	"github.com/teapotovh/teapot/lib/run"
	"github.com/teapotovh/teapot/service/ccm"
)

var (
	ErrAddressForNode = errors.New("could not generate random node local address")
)

type InternalIPConfig struct {
	Network netip.Prefix
}

func nodeInternalIP(prefix netip.Prefix, nodeName string) (netip.Addr, error) {
	hasher := md5.New()
	hash := [md5.Size]byte(hasher.Sum([]byte(nodeName)))

	bytes := prefix.Addr().AsSlice()
	for i := range 2 {
		bytes[2+i] = hash[i] ^ hash[2+i] ^ hash[4+i] ^ hash[6+i] ^ hash[8+i] ^ hash[10+i] ^ hash[12+i] ^ hash[14+i]
	}

	addr, ok := netip.AddrFromSlice(bytes)
	if !ok {
		return netip.IPv4Unspecified(), ErrAddressForNode
	}

	return addr, nil
}

type InternalIP struct {
	logger *slog.Logger
	ccm    *ccm.CCM

	prefix     netip.Prefix
	node       string
	internalIP netip.Addr
}

func NewInternalIP(ccm *ccm.CCM, config InternalIPConfig, logger *slog.Logger) (*InternalIP, error) {
	return &InternalIP{
		logger: logger,
		ccm:    ccm,

		prefix: config.Network,
	}, nil
}

func (iip *InternalIP) setInternalIP(ctx context.Context, ip netip.Addr, source string) error {
	if err := iip.ccm.SetInternalIP(ctx, ip); err != nil {
		return fmt.Errorf("error while updating node InternalIP (source: %s): %w", source, err)
	} else {
		iip.logger.Info("updated internal IP", "ip", ip, "old", iip.internalIP, "source", source)
		iip.internalIP = ip
		return nil
	}
}

// Run implements run.Runnable
func (iip *InternalIP) Run(ctx context.Context, notify run.Notify) error {
	sub := iip.ccm.Broker().Subscribe()
	defer sub.Unsubscribe()

	notify.Notify()
	for {
		select {
		case <-ctx.Done():
			return nil
		case event := <-sub.Chan():
			iip.logger.Debug("received CCM event", "event", event)

			if !event.InternalIP.IsValid() || iip.node == event.Node {
				iip.node = event.Node

				newIP, err := nodeInternalIP(iip.prefix, event.Node)
				if err != nil {
					return fmt.Errorf("error while computing local node IP: %w", err)
				}

				if err := iip.setInternalIP(ctx, newIP, "initial"); err != nil {
					return err
				}
			} else if iip.internalIP.IsValid() && event.InternalIP != iip.internalIP {
				// Ensure noone else tampers with InternalIP
				if err := iip.setInternalIP(ctx, iip.internalIP, "event"); err != nil {
					return err
				}
			}
		}
	}
}
