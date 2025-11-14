package wireguard

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"time"

	"github.com/vishvananda/netlink"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"github.com/teapotovh/teapot/lib/run"
	tnet "github.com/teapotovh/teapot/service/net"
)

const (
	WireguardKeepaliveInterval = time.Second * 15
)

type Wireguard struct {
	logger *slog.Logger
	net    *tnet.Net

	client *wgctrl.Client
	link   *netlink.Wireguard
	port   uint16

	cluster tnet.ClusterEvent
}

type WireguardConfig struct {
	Device string
}

func createInterface(name string) (*netlink.Wireguard, error) {
	prev, err := netlink.LinkByName(name)
	if err == nil {
		// remove the previous interface
		if err := netlink.LinkDel(prev); err != nil {
			return nil, fmt.Errorf("error while removing the previous interface: %w", err)
		}
	}
	// NOTE: it would be nice to nice to have a branch like
	// 	 elif !errors.Is(err, <not found error>) { ..
	//   return nil, fmt.Errorf("error while checking if link previously existed: %w", err)
	// but the library doesn't support reliable not found checks.

	link := &netlink.Wireguard{LinkAttrs: netlink.LinkAttrs{Name: name}}
	if err := netlink.LinkAdd(link); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, fmt.Errorf("failed to create wireguard device: %w", err)
	}
	if err := netlink.LinkSetUp(link); err != nil {
		return nil, fmt.Errorf("failed to bring up the wireguard device: %w", err)
	}

	return link, nil
}

func deleteInterface(link netlink.Link) error {
	return netlink.LinkDel(link)
}

func NewWireguard(net *tnet.Net, config WireguardConfig, logger *slog.Logger) (*Wireguard, error) {
	client, err := wgctrl.New()
	if err != nil {
		return nil, fmt.Errorf("error while creating wireguard client: %w", err)
	}

	link, err := createInterface(config.Device)
	if err != nil {
		return nil, fmt.Errorf("error while creating wireguard interface: %w", err)
	}

	wg := &Wireguard{
		logger: logger,
		net:    net,

		client: client,
		link:   link,
	}

	if err := wg.configureWireguard(); err != nil {
		return nil, fmt.Errorf("error while configuring wireguard interface: %w", err)
	}

	return wg, nil
}

func (w *Wireguard) configureWireguard() error {
	local := w.net.Local()
	privateKey := local.PrivateKey()
	port := int(w.port)

	interval := WireguardKeepaliveInterval
	var peers []wgtypes.PeerConfig
	for name, node := range w.cluster {
		if node.PublicKey == nil || node.ExternalAddress.Addr().IsUnspecified() {
			w.logger.Warn("ignoring node, as no wireguard key or endpoint are specified", "node", name)
			continue
		}

		endpoint, err := addrPortToUDPAddr(node.ExternalAddress)
		if err != nil {
			return fmt.Errorf("error while computing endpoint for node %q: %w", node, err)
		}

		// TODO: we should use internal IPs here, not cidrs
		var ips []net.IPNet
		for _, cidr := range node.CIDRs {
			ip, err := prefixToIPNet(cidr)
			if err != nil {
				return fmt.Errorf("error while computing allowed IP for node %q: %w", name, err)
			}
			ips = append(ips, *ip)
		}

		peers = append(peers, wgtypes.PeerConfig{
			PublicKey:                   *node.PublicKey,
			Endpoint:                    endpoint,
			PersistentKeepaliveInterval: &interval,
			// TOOD: should we?
			ReplaceAllowedIPs: true,
			AllowedIPs:        ips,
		})
	}

	return w.client.ConfigureDevice(w.link.Name, wgtypes.Config{
		PrivateKey:   &privateKey,
		ListenPort:   &port,
		ReplacePeers: true,
		Peers:        peers,
	})
}

func (w *Wireguard) updateCluster(cluster tnet.ClusterEvent) {
	w.cluster = cluster

}

// Run implements run.Runnable
func (w *Wireguard) Run(ctx context.Context, notify run.Notify) error {
	sub := w.net.Cluster().Broker().Subscribe()
	defer sub.Unsubscribe()
	// TODO: nicer, shared way to handle these errors
	defer logError(w.logger, w.client.Close)
	// TODO: handle error
	defer deleteInterface(w.link)

	notify.Notify()
	for event := range sub.Iter(ctx) {
		w.updateCluster(event)
		if err := w.configureWireguard(); err != nil {
			return fmt.Errorf("error while configuring wireguard interface: %w", err)
		}
	}

	return nil
}
