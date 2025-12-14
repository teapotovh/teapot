package wireguard

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"slices"

	"github.com/vishvananda/netlink"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

const (
	OptimalMTU = 1380
)

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

	link := &netlink.Wireguard{
		LinkAttrs: netlink.LinkAttrs{
			Name: name,
			MTU:  OptimalMTU,
		},
	}
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
		slices.EqualFunc(a.AllowedIPs, b.AllowedIPs, compareNetIP)
}

func compareUDPAddr(a, b net.UDPAddr) bool {
	return slices.Equal(a.IP, b.IP) && a.Port == b.Port
}

func compareNetIP(a, b net.IPNet) bool {
	return slices.Equal(a.IP, b.IP) && a.Network() == b.Network()
}
