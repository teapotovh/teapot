package internal

import (
	"net"
	"net/netip"
)

func PrefixToIPNet(p netip.Prefix) (*net.IPNet, error) {
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
