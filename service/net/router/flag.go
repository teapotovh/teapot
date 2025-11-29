package router

import (
	flag "github.com/spf13/pflag"
	"github.com/teapotovh/teapot/service/net/wireguard"
)

func RouterFlagSet() (*flag.FlagSet, func() RouterConfig) {
	fs := flag.NewFlagSet("net/router", flag.ExitOnError)

	device := fs.String("net-router-device", wireguard.DefaultWireguardDevice, "the device on which to route packets. It should be the same as the wireguard device")

	return fs, func() RouterConfig {
		return RouterConfig{
			Device: *device,
		}
	}
}
