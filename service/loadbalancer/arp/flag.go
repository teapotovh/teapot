package arp

import (
	flag "github.com/spf13/pflag"
)

func ARPFlagSet() (*flag.FlagSet, func() ARPConfig) {
	fs := flag.NewFlagSet("loadbalancer/arp", flag.ExitOnError)

	device := fs.String("loadbalancer-arp-device", "eth0", "the interface on which to broadcast ARP packets")

	return fs, func() ARPConfig {
		return ARPConfig{
			Device: *device,
		}
	}
}
