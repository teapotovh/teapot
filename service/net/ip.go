package net

import (
	"crypto/md5"
	"errors"
	"net/netip"
)

var (
	ErrAddressForNode = errors.New("could not generate random node local address")

	// This is the first /16 subnet out of the whole /10 net allocated for
	// CG-NAT communication. We take /16 so that the remainder of the address can
	// be filled with two bytes taken from XORing the MD5 of hostnames.
	InternalPrefix = netip.MustParsePrefix("100.64.0.0/16")
)

func NodeInternalIP(nodeName string) (netip.Addr, error) {
	hasher := md5.New()
	hash := [md5.Size]byte(hasher.Sum([]byte(nodeName)))

	bytes := InternalPrefix.Addr().AsSlice()
	for i := range 2 {
		bytes[2+i] = hash[i] ^ hash[2+i] ^ hash[4+i] ^ hash[6+i] ^ hash[8+i] ^ hash[10+i] ^ hash[12+i] ^ hash[14+i]
	}

	addr, ok := netip.AddrFromSlice(bytes)
	if !ok {
		return netip.IPv4Unspecified(), ErrAddressForNode
	}

	return addr, nil
}
