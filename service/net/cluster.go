package net

import (
	"context"
	"fmt"
	"log/slog"
	"net/netip"

	"github.com/teapotovh/teapot/lib/broker"
	"github.com/teapotovh/teapot/lib/run"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

type ClusterConfig struct {
	LocalNode string
}

type Cluster struct {
	logger *slog.Logger
	net    *Net

	node  string
	state map[string]ClusterNode

	broker       *broker.Broker[ClusterEvent]
	brokerCancel context.CancelFunc
}

type ClusterNode struct {
	ExternalAddress netip.AddrPort
	PublicKey       *wgtypes.Key

	IsLocal    bool
	InternalIP netip.Addr
	CIDRs      []netip.Prefix
}

type ClusterEvent map[string]ClusterNode

func NewCluster(net *Net, config ClusterConfig, logger *slog.Logger) (*Cluster, error) {
	ctx, cancel := context.WithCancel(context.Background())
	broker := broker.NewBroker[ClusterEvent]()
	go broker.Run(ctx)

	return &Cluster{
		logger: logger,
		net:    net,

		node:  config.LocalNode,
		state: make(map[string]ClusterNode),

		broker:       broker,
		brokerCancel: cancel,
	}, nil
}

func (c *Cluster) Broker() *broker.Broker[ClusterEvent] {
	return c.broker
}

func (c *Cluster) toClusterNode(node Node) (ClusterNode, error) {
	internalIP, err := NodeInternalIP(node.Name)
	if err != nil {
		return ClusterNode{}, fmt.Errorf("could not convert Node to ClusterNode: %w", err)
	}

	// This CNI is ipv6-only, so filter only ipv6 addresses
	var cidrs []netip.Prefix
	for _, cidr := range node.CIDRs {
		if cidr.Addr().Is6() {
			cidrs = append(cidrs, cidr)
		}
	}

	return ClusterNode{
		ExternalAddress: node.ExternalAddress,
		PublicKey:       node.PublicKey,

		IsLocal:    node.Name == c.node,
		InternalIP: internalIP,
		CIDRs:      cidrs,
	}, nil
}

// Run implements run.Runnable
func (c *Cluster) Run(ctx context.Context, notify run.Notify) error {
	defer c.brokerCancel()
	sub := c.net.broker.Subscribe()
	defer sub.Unsubscribe()

	notify.Notify()
	for event := range sub.Iter(ctx) {
		if event.Delete != nil {
			name := *event.Delete
			delete(c.state, name)
		} else if event.Update != nil {
			node := *event.Update
			clusterNode, err := c.toClusterNode(node)
			if err != nil {
				return fmt.Errorf("error getting ClusterNode for event: %w", err)
			}

			c.state[node.Name] = clusterNode

			l := c.logger
			for name, node := range c.state {
				f := func(field string) string {
					return fmt.Sprintf("(%s).%s", name, field)
				}
				l = l.With(f("externalAddress"), node.ExternalAddress)
				l = l.With(f("publicKey"), node.PublicKey)
				l = l.With(f("internalIP"), node.InternalIP)
				l = l.With(f("cidrs"), node.CIDRs)
			}
			l.Debug("updated cluster state")

			c.broker.Publish(c.state)
		}
	}

	return nil
}
