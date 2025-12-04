package net

import (
	"fmt"
	"net/netip"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	v1 "k8s.io/api/core/v1"
)

const (
	AnnotationExternalPort = "net.teapot.ovh/external-port"
	AnnotationPublicKey    = "net.teapot.ovh/public-key"

	DefaultWireguardPort = uint16(51692)
)

type Event struct {
	Update *Node
	Delete *string
}

type Node struct {
	Name  string
	CIDRs []netip.Prefix

	InternalAddress netip.Addr
	ExternalAddress netip.AddrPort
	PublicKey       *wgtypes.Key
}

func (net *Net) handle(name string, n *v1.Node, exists bool) error {
	if !exists {
		net.logger.Debug("node was removed", "name", name)
		net.broker.Publish(Event{Delete: &name})
		return nil
	}

	var cidrs []netip.Prefix
	for _, cidr := range n.Spec.PodCIDRs {
		prefix, err := netip.ParsePrefix(cidr)
		if err != nil {
			return fmt.Errorf("error while parsing CIDR %q as network prefix: %w", cidr, err)
		}
		cidrs = append(cidrs, prefix)
	}

	var (
		externalIP, internalIP netip.Addr
		err                    error
	)
	for _, addr := range n.Status.Addresses {
		switch addr.Type {
		case v1.NodeInternalIP:
			internalIP, err = netip.ParseAddr(addr.Address)
			if err != nil {
				return fmt.Errorf("error while parsing the internal ip %q for node %s: %w", addr.Address, n.Name, err)
			}
		case v1.NodeExternalIP:
			externalIP, err = netip.ParseAddr(addr.Address)
			if err != nil {
				return fmt.Errorf("error while parsing the external ip %q for node %s: %w", addr.Address, n.Name, err)
			}
		}
	}

	rawPort, ok := n.Annotations[AnnotationExternalPort]
	port := DefaultWireguardPort
	if ok {
		_, err := fmt.Sscanf(rawPort, "%d", &port)
		if err != nil {
			return fmt.Errorf("error while parsing the external port %q for node %s: %w", rawPort, n.Name, err)
		}
	}

	rawPublicKey, ok := n.Annotations[AnnotationPublicKey]
	var publicKey *wgtypes.Key
	if ok {
		pk, err := wgtypes.ParseKey(rawPublicKey)
		if err != nil {
			return fmt.Errorf("error while parsing the public key %q for node %s: %w", rawPublicKey, n.Name, err)
		}
		publicKey = &pk
	}

	addr := netip.AddrPortFrom(externalIP, port)
	node := Node{
		Name:  n.Name,
		CIDRs: cidrs,

		InternalAddress: internalIP,
		ExternalAddress: addr,
		PublicKey:       publicKey,
	}

	net.logger.Info("received node update", "name", node.Name, "cidrs", node.CIDRs, "externalAddress", node.ExternalAddress, "internalAddress", node.InternalAddress, "publicKey", node.PublicKey)
	net.broker.Publish(Event{Update: &node})

	return nil
}
