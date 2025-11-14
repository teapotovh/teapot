package wireguard

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"slices"
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

	// State for the currently configured interface, to avoid unnecessary updates
	peers      []wgtypes.PeerConfig
	privateKey wgtypes.Key
	port       uint16

	cluster tnet.ClusterEvent
	local   tnet.LocalEvent
}

type WireguardConfig struct {
	Device string
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

	return wg, nil
}

func (w *Wireguard) configureWireguard(source string) error {
	if w.local.PrivateKey == tnet.DefaultPrivateKey || w.local.Port == 0 {
		w.logger.Warn("ignoring update, as information for local node hasn't been fetched yet", "source", source)
		return nil
	}

	interval := WireguardKeepaliveInterval
	var newPeers []wgtypes.PeerConfig
	for name, node := range w.cluster {
		if node.IsLocal {
			continue
		}

		if node.PublicKey == nil || node.ExternalAddress.Addr().IsUnspecified() {
			w.logger.Warn("ignoring node, as no wireguard key or endpoint are specified", "node", name)
			continue
		}

		endpoint, err := addrPortToUDPAddr(node.ExternalAddress)
		if err != nil {
			return fmt.Errorf("error while computing endpoint for node %q: %w", name, err)
		}

		ip, err := prefixToIPNet(netip.PrefixFrom(node.InternalIP, 128))
		if err != nil {
			return fmt.Errorf("error while computing allowed IP for node %q: %w", name, err)
		}
		ips := []net.IPNet{*ip}

		newPeers = append(newPeers, wgtypes.PeerConfig{
			PublicKey:                   *node.PublicKey,
			Endpoint:                    endpoint,
			PersistentKeepaliveInterval: &interval,
			ReplaceAllowedIPs:           true,
			AllowedIPs:                  ips,
		})
	}

	if slices.EqualFunc(newPeers, w.peers, comparePeerConfig) && w.privateKey == w.local.PrivateKey && w.port == w.local.Port {
		w.logger.Debug("update caused no change in wireguard config", "source", source)
		return nil
	}
	w.peers = newPeers
	w.privateKey = w.local.PrivateKey
	w.port = w.local.Port

	port := int(w.local.Port)
	err := w.client.ConfigureDevice(w.link.Name, wgtypes.Config{
		PrivateKey:   &w.local.PrivateKey,
		ListenPort:   &port,
		ReplacePeers: true,
		Peers:        newPeers,
	})
	if err != nil {
		return fmt.Errorf("error while configuring wireguard device: %w", err)
	}

	w.logger.Info("updated wireguard with new information", "source", source)
	return nil
}

// Run implements run.Runnable
func (w *Wireguard) Run(ctx context.Context, notify run.Notify) error {
	csub := w.net.Cluster().Broker().Subscribe()
	defer csub.Unsubscribe()
	lsub := w.net.Local().Broker().Subscribe()
	defer lsub.Unsubscribe()

	// TODO: nicer, shared way to handle these errors
	defer logError(w.logger, w.client.Close)
	// TODO: handle error
	defer deleteInterface(w.link)

	notify.Notify()
	for {
		select {
		case <-ctx.Done():
			return nil
		case cluster := <-csub.Chan():
			w.cluster = cluster

			if err := w.configureWireguard("cluster"); err != nil {
				return fmt.Errorf("error while configuring wireguard interface: %w", err)
			}

		case local := <-lsub.Chan():
			w.local = local

			if err := w.configureWireguard("local"); err != nil {
				return fmt.Errorf("error while configuring wireguard interface: %w", err)
			}
		}
	}
}
