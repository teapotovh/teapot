package cni

import (
	flag "github.com/spf13/pflag"
)

func CNIFlagSet() (*flag.FlagSet, func() CNIConfig) {
	fs := flag.NewFlagSet("net/cni", flag.ExitOnError)

	device := fs.String("net-cni-device", "cni0", "the CNI device name to use for the bridge interface")
	path := fs.String("net-cni-path", "/etc/cni/net.d", "the path to where the CNI configuration should be placed")

	return fs, func() CNIConfig {
		return CNIConfig{
			Device: *device,
			Path:   *path,
		}
	}
}
