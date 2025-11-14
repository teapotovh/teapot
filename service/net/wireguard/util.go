package wireguard

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"slices"

	"github.com/vishvananda/netlink"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
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

func createInterface(name string) (*netlink.Wireguard, error) {
	prev, err := netlink.LinkByName(name)
	if err == nil {
		// remove the previous interface
		if err := netlink.LinkDel(prev); err != nil {
			return nil, fmt.Errorf("error while removing the previous interface: %w", err)
		}
	}
	// NOTE: it would be nice to nice to have a branch like
	// 	 elif !errors.Is(err, <not found error>) { ..
	//   return nil, fmt.Errorf("error while checking if link previously existed: %w", err)
	// but the library doesn't support reliable not found checks.

	link := &netlink.Wireguard{LinkAttrs: netlink.LinkAttrs{Name: name}}
	if err := netlink.LinkAdd(link); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, fmt.Errorf("failed to create wireguard device: %w", err)
	}
	if err := netlink.LinkSetUp(link); err != nil {
		return nil, fmt.Errorf("failed to bring up the wireguard device: %w", err)
	}

	return link, nil
}

func deleteInterface(link netlink.Link) error {
	return netlink.LinkDel(link)
}

func comparePeerConfig(a, b wgtypes.PeerConfig) bool {
	return a.PublicKey == b.PublicKey &&
		compareUDPAddr(*a.Endpoint, *b.Endpoint) &&
		slices.EqualFunc(a.AllowedIPs, b.AllowedIPs, compareNetIp)
}

func compareUDPAddr(a, b net.UDPAddr) bool {
	return slices.Equal(a.IP, b.IP) && a.Port == b.Port
}

func compareNetIp(a, b net.IPNet) bool {
	return slices.Equal(a.IP, b.IP) && a.Network() == b.Network()
}
