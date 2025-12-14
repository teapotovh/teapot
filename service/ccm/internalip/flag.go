package internalip

import (
	"fmt"
	"net"
	"net/netip"

	flag "github.com/spf13/pflag"
)

// InternalPrefix is the first /16 subnet out of the whole /10 net allocated for
// CG-NAT communication. We take /16 so that the remainder of the address can
// be filled with two bytes taken from XORing the MD5 of hostnames.
var InternalPrefix net.IPNet

//nolint:gochecknoinits
func init() {
	_, net, err := net.ParseCIDR("100.64.0.0/16")
	if err != nil {
		panic(fmt.Errorf("error while parsing built default internal prefix: %w", err))
	}

	InternalPrefix = *net
}

func ipNetToPrefix(net net.IPNet) netip.Prefix {
	bits, _ := net.Mask.Size()
	return netip.PrefixFrom(netip.MustParseAddr(net.IP.String()), bits)
}

func InternalIPFlagSet() (*flag.FlagSet, func() InternalIPConfig) {
	fs := flag.NewFlagSet("ccm/externalip", flag.ExitOnError)

	network := fs.IPNet("ccm-internalip-network", InternalPrefix, "the range from which to allocate node local IPs")

	return fs, func() InternalIPConfig {
		return InternalIPConfig{
			Network: ipNetToPrefix(*network),
		}
	}
}
