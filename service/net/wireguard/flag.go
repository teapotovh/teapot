package wireguard

import (
	flag "github.com/spf13/pflag"
)

const (
	DefaultWireguardDevice = "teapotnet0"
)

func WireguardFlagSet() (*flag.FlagSet, func() WireguardConfig) {
	fs := flag.NewFlagSet("net/wireguard", flag.ExitOnError)

	device := fs.String(
		"net-wireguard-device",
		DefaultWireguardDevice,
		"the wireguard device name to use for the mesh interface",
	)

	return fs, func() WireguardConfig {
		return WireguardConfig{
			Device: *device,
		}
	}
}
