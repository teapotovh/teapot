package net

import (
	"context"
	"fmt"
	"net"
	"net/netip"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
)

func annotateNode(
	ctx context.Context,
	clientset *kubernetes.Clientset,

	nodeName string,
	key string,
	value string,
) error {
	const annotationKey = "net.teapot.ovh/public-key"

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		node, err := clientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get kubernetes node %q: %w", nodeName, err)
		}

		if node.Annotations == nil {
			node.Annotations = make(map[string]string)
		}

		node.Annotations[key] = value

		if _, err := clientset.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("failed to update kubernetes node %q: %w", node.Name, err)
		}

		return nil
	})
}

func addrPortToUDPAddr(ap netip.AddrPort) (*net.UDPAddr, error) {
	if !ap.IsValid() {
		return nil, net.InvalidAddrError("invalid AddrPort")
	}
	ip := ap.Addr()
	if !ip.IsValid() {
		return nil, net.InvalidAddrError("invalid IP address")
	}
	return &net.UDPAddr{
		IP:   ip.AsSlice(),
		Port: int(ap.Port()),
		Zone: ip.Zone(),
	}, nil
}

func prefixToIPNet(p netip.Prefix) (*net.IPNet, error) {
	if !p.IsValid() {
		return nil, net.InvalidAddrError("invalid Prefix")
	}
	ip := p.Addr()
	if !ip.IsValid() {
		return nil, net.InvalidAddrError("invalid IP address in Prefix")
	}
	return &net.IPNet{
		IP:   ip.AsSlice(),
		Mask: net.CIDRMask(p.Bits(), ip.BitLen()),
	}, nil
}
