package net

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	"github.com/vishvananda/netlink"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

type Wireguard struct {
	logger *slog.Logger
	net    *Net

	client *wgctrl.Client
	node   string
	device string
	link   *netlink.Wireguard
	port   uint16

	nodes map[string]Node
}

type WireguardConfig struct {
	LocalNode string
	Device    string
}

func NewWireguard(net *Net, config WireguardConfig, logger *slog.Logger) (*Wireguard, error) {
	client, err := wgctrl.New()
	if err != nil {
		return nil, fmt.Errorf("error while creating wireguard client: %w", err)
	}

	wg := &Wireguard{
		logger: logger,
		net:    net,

		client: client,
		node:   config.LocalNode,
		device: config.Device,
		link:   &netlink.Wireguard{LinkAttrs: netlink.LinkAttrs{Name: config.Device}},
		port:   DefaultWireguardPort,

		nodes: map[string]Node{},
	}

	if err := wg.createInterface(); err != nil {
		return nil, fmt.Errorf("error while creating wireguard interface: %w", err)
	}

	if err := wg.configureWireguard(); err != nil {
		return nil, fmt.Errorf("error while configuring wireguard interface: %w", err)
	}

	return wg, nil
}

func (l *Wireguard) createInterface() error {
	if err := netlink.LinkAdd(l.link); err != nil {
		return fmt.Errorf("failed to create wireguard device: %w", err)
	}
	if err := netlink.LinkSetUp(l.link); err != nil {
		return fmt.Errorf("failed to bring up the wireguard device: %w", err)
	}

	return nil
}

func (l *Wireguard) deleteInterface() error {
	return netlink.LinkDel(l.link)
}

func (l *Wireguard) configureWireguard() error {
	local := l.net.Local()
	privateKey := local.PrivateKey()
	port := int(l.port)

	interval := WireguardKeepaliveInterval
	var peers []wgtypes.PeerConfig
	for _, node := range l.nodes {
		if node.PublicKey == nil || node.Address.Addr().IsUnspecified() {
			l.logger.Warn("ignoring node, as no wireguard key or endpoint are specified", "node", node.Name)
			continue
		}

		endpoint, err := addrPortToUDPAddr(node.Address)
		if err != nil {
			return fmt.Errorf("error while computing endpoint for node %q: %w", node.Name, err)
		}

		// TODO: we should use internal IPs here, not cidrs
		var ips []net.IPNet
		for _, cidr := range node.CIDRs {
			ip, err := prefixToIPNet(cidr)
			if err != nil {
				return fmt.Errorf("error while computing allowed IP for node %q: %w", node.Name, err)
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

	return l.client.ConfigureDevice(l.device, wgtypes.Config{
		PrivateKey:   &privateKey,
		ListenPort:   &port,
		ReplacePeers: true,
		Peers:        peers,
	})
}

func (l *Wireguard) Run(ctx context.Context) {
	sub := l.net.Broker().Subscribe()
	defer sub.Unsubscribe()
	defer l.client.Close()
	defer l.deleteInterface()

	for event := range sub.Iter(ctx) {
		if event.Delete != nil {
			name := *event.Delete
			l.logger.Info("deleting wireguard node", "name", name)
			delete(l.nodes, name)
		} else if event.Update != nil {
			node := *event.Update
			l.logger.Info("got node update", "node", node)

			l.nodes[node.Name] = node

			if err := l.configureWireguard(); err != nil {
				l.logger.Error("error while configuring wireguard interface", "err", err)
			}
		}
	}
}
