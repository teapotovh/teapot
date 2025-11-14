package wireguard

import (
	"log/slog"
	"net"
	"net/netip"
)

func logError(logger *slog.Logger, fn func() error) {
	if err := fn(); err != nil {
		logger.Error("error in defer call", "err", err)
	}
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
