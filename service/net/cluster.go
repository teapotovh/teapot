package net

import (
	"fmt"
	"log/slog"
	"net/netip"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"github.com/teapotovh/teapot/lib/broker"
	"github.com/teapotovh/teapot/lib/run"
)

type ClusterConfig struct {
	LocalNode string
}

type Cluster struct {
	logger *slog.Logger
	net    *Net
	state  map[string]ClusterNode
	broker *broker.Broker[ClusterEvent]
	node   string
}

type ClusterNode struct {
	InternalAddress netip.Addr
	PublicKey       *wgtypes.Key
	ExternalAddress netip.AddrPort
	CIDRs           []netip.Prefix
	IsLocal         bool
}

type ClusterEvent map[string]ClusterNode

func NewCluster(net *Net, config ClusterConfig, logger *slog.Logger) (*Cluster, error) {
	return &Cluster{
		logger: logger,
		net:    net,

		node:  config.LocalNode,
		state: make(map[string]ClusterNode),

		broker: broker.NewBroker[ClusterEvent](),
	}, nil
}

func (c *Cluster) Broker() *broker.Broker[ClusterEvent] {
	return c.broker
}

// Run implements run.Runnable.
func (c *Cluster) Run(step run.Step) error {
	step.Add("broker", c.broker)
	step.Add("events", clusterEvents{c})
	step.Wait()
	step.Notify()

	<-step.Context().Done()

	return nil
}

type clusterEvents struct{ *Cluster }

// Run implements run.Runnable.
func (c clusterEvents) Run(step run.Step) error {
	ctx := step.Context()

	sub := c.net.broker.Subscribe()
	defer sub.Unsubscribe()

	step.Notify()

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

			c.broker.Publish(c.state)
		}
	}

	return nil
}

func (c *Cluster) toClusterNode(node Node) (ClusterNode, error) {
	// This CNI is ipv4-only, so filter only ipv4 addresses
	var cidrs []netip.Prefix

	for _, cidr := range node.CIDRs {
		if cidr.Addr().Is4() {
			cidrs = append(cidrs, cidr)
		}
	}

	return ClusterNode{
		InternalAddress: node.InternalAddress,
		ExternalAddress: node.ExternalAddress,
		PublicKey:       node.PublicKey,

		IsLocal: node.Name == c.node,
		CIDRs:   cidrs,
	}, nil
}
