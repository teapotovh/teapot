package net

import (
	"crypto/md5"
	"fmt"
	"net/netip"
)

var (
	ULA = netip.MustParsePrefix("fdfa:debc:e9ad::/48")
	// The first /64 subnet of the ULA randomly selected for teapot's netd
	// is the network where internal node IPs live (used for WireGuard / BGP).
	InternalPrefix = netip.PrefixFrom(ULA.Addr(), 96)
)

func NodeInternalIP(nodeName string) (netip.Addr, error) {
	hasher := md5.New()
	hash := [md5.Size]byte(hasher.Sum([]byte(nodeName)))

	bytes := InternalPrefix.Addr().AsSlice()
	for i := range 4 {
		bytes[12+i] = hash[i] ^ hash[4+i] ^ hash[8+i] ^ hash[12+i]
	}

	addr, ok := netip.AddrFromSlice(bytes)
	if !ok {
		return netip.IPv6Unspecified(), fmt.Errorf("could not generate random node local address")
	}

	return addr, nil
}
