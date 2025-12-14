package cni

import (
	"errors"
	"fmt"
	"net/netip"
	"os"

	"github.com/vishvananda/netlink"

	"github.com/teapotovh/teapot/service/net/wireguard"
)

var (
	ErrCNIInterfaceNotBridge = errors.New("cni interface is not of type bridge")
	allRoutes                = netip.PrefixFrom(netip.IPv4Unspecified(), 0)
)

func createInterface(name string) (*netlink.Bridge, error) {
	prev, err := netlink.LinkByName(name)
	if err == nil {
		if prev.Type() != (&netlink.Bridge{}).Type() {
			return nil, ErrCNIInterfaceNotBridge
		}

		return prev.(*netlink.Bridge), nil
	}

	link := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: name}}
	if err := netlink.LinkAdd(link); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, fmt.Errorf("failed to create bridge device: %w", err)
	}
	if err := netlink.LinkSetUp(link); err != nil {
		return nil, fmt.Errorf("failed to bring up the bridge device: %w", err)
	}

	return link, nil
}

func deleteInterface(link netlink.Link) error {
	return netlink.LinkDel(link)
}

type cniConfigList struct {
	CNIVersion string         `json:"cniVersion"`
	Name       string         `json:"name"`
	Plugins    []bridgePlugin `json:"plugins"`
}

type bridgePlugin struct {
	Type      string        `json:"type"`
	Bridge    string        `json:"bridge,omitempty"`
	IsGateway bool          `json:"isGateway,omitempty"`
	IPMasq    bool          `json:"ipMasq,omitempty"`
	MTU       int           `json:"mtu,omitempty"`
	IPAM      hostLocalIPAM `json:"ipam,omitempty"`
}

type hostLocalIPAM struct {
	Type   string                 `json:"type"`
	Ranges [][]hostLocalIPAMRange `json:"ranges"`
	Routes []hostLocalIPAMRoute   `json:"routes"`
}

type hostLocalIPAMRange struct {
	Subnet  string `json:"subnet"`
	Gateway string `json:"gateway,omitempty"`
}

type hostLocalIPAMRoute struct {
	Dst string `json:"dst"`
}

func cniConfig(device string, cidrs []netip.Prefix) cniConfigList {
	config := cniConfigList{
		CNIVersion: "0.4.0",
		Name:       "teapotnet",
	}

	bridgePlugin := bridgePlugin{
		Type:      "bridge",
		Bridge:    device,
		IsGateway: true,
		IPMasq:    false,
		MTU:       wireguard.OptimalMTU,
		IPAM: hostLocalIPAM{
			Type: "host-local",
		},
	}
	for _, cidr := range cidrs {
		bridgePlugin.IPAM.Ranges = append(bridgePlugin.IPAM.Ranges, []hostLocalIPAMRange{
			{
				Subnet:  cidr.String(),
				Gateway: cidr.Addr().Next().String(),
			},
		})
	}
	bridgePlugin.IPAM.Routes = []hostLocalIPAMRoute{{Dst: allRoutes.String()}}

	config.Plugins = append(config.Plugins, bridgePlugin)
	return config
}
