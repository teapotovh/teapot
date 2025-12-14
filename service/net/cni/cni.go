package cni

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/coreos/go-iptables/iptables"
	"github.com/vishvananda/netlink"

	"github.com/teapotovh/teapot/lib/run"
	tnet "github.com/teapotovh/teapot/service/net"
	"github.com/teapotovh/teapot/service/net/internal"
)

const (
	CNIFilename = "10-teapotnet.conflist"
	CNIPerm     = os.FileMode(0o664)
)

type rule struct {
	table string
	chain string
	rule  []string
}

func (r rule) String() string {
	return fmt.Sprintf("iptables -t %s -A %s %s", r.table, r.chain, strings.Join(r.rule, " "))
}

type CNI struct {
	logger *slog.Logger
	net    *tnet.Net

	link      netlink.Link
	cniConfig []byte
	cniPath   string

	iptables *iptables.IPTables
	rules    []rule

	cluster   tnet.ClusterEvent
	local     tnet.LocalEvent
	localNode tnet.ClusterNode
}

type CNIConfig struct {
	Device string
	Path   string
}

func NewCNI(net *tnet.Net, config CNIConfig, logger *slog.Logger) (*CNI, error) {
	link, err := createInterface(config.Device)
	if err != nil {
		return nil, fmt.Errorf("error while creating CNI interface: %w", err)
	}

	path := filepath.Clean(config.Path)
	if err := os.MkdirAll(path, os.ModePerm); err != nil {
		return nil, fmt.Errorf("error while ensuring local CNI directory exists: %w", err)
	}

	ipt, err := iptables.New()
	if err != nil {
		return nil, fmt.Errorf("error while creating iptables client: %w", err)
	}

	cniPath := filepath.Join(path, CNIFilename)
	return &CNI{
		logger: logger,
		net:    net,

		link:     link,
		iptables: ipt,
		cniPath:  cniPath,
	}, nil
}

func (c *CNI) addCNIIP(source string) error {
	addrs, err := netlink.AddrList(c.link, netlink.FAMILY_V4)
	if err != nil {
		return fmt.Errorf("error while listing addresses for the CNI interface: %w", err)
	}

	for _, cidr := range c.localNode.CIDRs {
		// The CNI bridge interface always gets the first IP of the range
		ip := cidr.Addr().Next()

		for _, a := range addrs {
			if a.IP.Equal(ip.AsSlice()) {
				c.logger.Debug("CNI interface already has CIDR IP, skipping", "cidr", cidr, "source", source)
				return nil
			}
		}

		nip, err := internal.PrefixToIPNet(netip.PrefixFrom(ip, cidr.Bits()))
		if err != nil {
			return fmt.Errorf("error while computing bridge IP for CIDR %q: %w", cidr, err)
		}

		addr := &netlink.Addr{IPNet: nip}
		if err := netlink.AddrAdd(c.link, addr); err != nil {
			return fmt.Errorf("error while adding bridge IP for CIDR %q: %w", cidr, err)
		}
	}

	return nil
}

func (c *CNI) writeCNIConfig(source string) error {
	config := cniConfig(c.link.Attrs().Name, c.localNode.CIDRs)

	// Use pretty JSON formatting for debuggability
	rawConfig, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("error while marshaling CNI config: %w", err)
	}

	if slices.Equal(rawConfig, c.cniConfig) {
		c.logger.Debug("update caused no change in cni config", "source", source)
		return nil
	}

	if err := os.WriteFile(c.cniPath, rawConfig, CNIPerm); err != nil {
		return fmt.Errorf("error while writing CNI config at %q: %w", c.cniPath, err)
	}

	c.cniConfig = rawConfig
	return nil
}

func (c *CNI) addCNIRules(source string) error {
	var rules []rule

	for _, cidr := range c.localNode.CIDRs {
		rules = append(rules, rule{"filter", "FORWARD", []string{
			"-s",
			cidr.String(),
			"-j",
			"ACCEPT",
		}})
		rules = append(rules, rule{"filter", "FORWARD", []string{
			"-d",
			cidr.String(),
			"-j",
			"ACCEPT",
		}})
		rules = append(rules, rule{"nat", "POSTROUTING", []string{
			"-s",
			cidr.String(),
			"!",
			"-d",
			cidr.String(),
			"-j",
			"MASQUERADE",
		}})
	}

	for _, r := range rules {
		if err := c.iptables.AppendUnique(r.table, r.chain, r.rule...); err != nil {
			return fmt.Errorf("error while adding iptables rule %q: %w", r, err)
		}
	}

	c.rules = rules
	return nil
}

func (c *CNI) configureCNI(source string) error {
	if c.local.Node == "" {
		c.logger.Warn("ignoring update, as information for local node hasn't been fetched yet", "source", source)
		return nil
	}

	// We need to fetch the local node from the cluster to get its CIDRs
	found := false
	for _, n := range c.cluster {
		if n.IsLocal {
			c.localNode = n
			found = true
			break
		}
	}
	if !found {
		c.logger.Warn("could not configure CNI as local node is not available in the cluster", "source", source)
		return nil
	}

	if err := c.addCNIIP(source); err != nil {
		return fmt.Errorf("error while adding IPs to the CNI interface: %w", err)
	}

	if err := c.writeCNIConfig(source); err != nil {
		return fmt.Errorf("error while writing CNI configuration: %w", err)
	}

	if err := c.addCNIRules(source); err != nil {
		return fmt.Errorf("error while adding CNI iptables rules: %w", err)
	}

	return nil
}

func (c *CNI) cleanupCNIFile() error {
	if len(c.cniConfig) > 0 {
		if err := os.Remove(c.cniPath); err != nil {
			return fmt.Errorf("error while removing CNI configuration: %w", err)
		}
	}

	return nil
}

func (c *CNI) cleanupIptables() error {
	for _, r := range c.rules {
		if err := c.iptables.Delete(r.table, r.chain, r.rule...); err != nil {
			return fmt.Errorf("error while removing stale iptables rule %q: %s", r, err)
		}
	}

	return nil
}

// Run implements run.Runnable
func (c *CNI) Run(ctx context.Context, notify run.Notify) error {
	csub := c.net.Cluster().Broker().Subscribe()
	defer csub.Unsubscribe()
	lsub := c.net.Local().Broker().Subscribe()
	defer lsub.Unsubscribe()

	// TODO: handle error
	defer deleteInterface(c.link)
	// TODO: handle error
	defer c.cleanupCNIFile()
	// TODO: handle error
	defer c.cleanupIptables()

	notify.Notify()
	for {
		select {
		case <-ctx.Done():
			return nil
		case cluster := <-csub.Chan():
			c.cluster = cluster

			if err := c.configureCNI("cluster"); err != nil {
				return fmt.Errorf("error while configuring wireguard interface: %w", err)
			}

		case local := <-lsub.Chan():
			c.local = local

			if err := c.configureCNI("local"); err != nil {
				return fmt.Errorf("error while configuring wireguard interface: %w", err)
			}
		}
	}
}
