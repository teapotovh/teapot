package wireguard

import (
	"bytes"
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
	"github.com/teapotovh/teapot/service/net/internal"
)

const (
	WireguardKeepaliveInterval = time.Second * 15
	NodePrefix                 = 32
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

func (w *Wireguard) addWireguardIP() error {
	// We need to fetch the local node from the cluster to get its internal IP
	var node *tnet.ClusterNode
	for _, n := range w.cluster {
		if n.IsLocal {
			node = &n
			break
		}
	}
	if node == nil {
		w.logger.Warn("could not configure IP on wireguard interface as local node is not available in the cluster")
		return nil
	}

	addrs, err := netlink.AddrList(w.link, netlink.FAMILY_V4)
	if err != nil {
		return fmt.Errorf("error while listing addresses for the wireguard interface: %w", err)
	}

	for _, a := range addrs {
		if a.IPNet.IP.Equal(node.InternalAddress.AsSlice()) {
			w.logger.Debug("wireguard interface already has local IP, skipping", "ip", node.InternalAddress)
			return nil
		}
	}

	ip, err := internal.PrefixToIPNet(netip.PrefixFrom(node.InternalAddress, NodePrefix))
	if err != nil {
		return fmt.Errorf("error while computing local node IP: %w", err)
	}

	addr := &netlink.Addr{IPNet: ip}
	if err := netlink.AddrAdd(w.link, addr); err != nil {
		return fmt.Errorf("error while adding local node IP: %w", err)
	}

	return nil
}

func (w *Wireguard) configureWireguard(source string) error {
	if w.local.PrivateKey == tnet.DefaultPrivateKey || w.local.Port == 0 {
		w.logger.Warn("ignoring update, as information for local node hasn't been fetched yet", "source", source)
		return nil
	}

	if err := w.addWireguardIP(); err != nil {
		return fmt.Errorf("error while adding local node IP to wireguard interface: %w", err)
	}

	interval := WireguardKeepaliveInterval
	var newPeers []wgtypes.PeerConfig
	for name, node := range w.cluster {
		if node.IsLocal {
			continue
		}

		if node.PublicKey == nil || !node.ExternalAddress.Addr().IsValid() {
			w.logger.Warn("ignoring node, as no wireguard key or endpoint are specified", "node", name)
			continue
		}

		endpoint, err := addrPortToUDPAddr(node.ExternalAddress)
		if err != nil {
			return fmt.Errorf("error while computing endpoint for node %q: %w", name, err)
		}

		ip, err := internal.PrefixToIPNet(netip.PrefixFrom(node.InternalAddress, NodePrefix))
		if err != nil {
			return fmt.Errorf("error while computing allowed IP for node %q: %w", name, err)
		}
		ips := []net.IPNet{*ip}
		for _, c := range node.CIDRs {
			cidr, err := internal.PrefixToIPNet(c)
			if err != nil {
				return fmt.Errorf("error while computing allowed IP for node %q from CIDR %q: %w", name, c, err)
			}
			ips = append(ips, *cidr)
		}

		newPeers = append(newPeers, wgtypes.PeerConfig{
			PublicKey:                   *node.PublicKey,
			Endpoint:                    endpoint,
			PersistentKeepaliveInterval: &interval,
			ReplaceAllowedIPs:           true,
			AllowedIPs:                  ips,
		})
	}

	slices.SortFunc(newPeers, func(a, b wgtypes.PeerConfig) int {
		return bytes.Compare(a.PublicKey[:], b.PublicKey[:])
	})

	if slices.EqualFunc(newPeers, w.peers, comparePeerConfig) && w.privateKey == w.local.PrivateKey &&
		w.port == w.local.Port {
		w.logger.Debug("update caused no change in wireguard config", "source", source)
		return nil
	}
	w.peers = newPeers
	w.privateKey = w.local.PrivateKey
	w.port = w.local.Port

	w.logger.Debug("applying peers", "peers", newPeers)
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
	defer w.client.Close()
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
