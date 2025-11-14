package bgp

import (
	flag "github.com/spf13/pflag"
	"github.com/teapotovh/teapot/service/net/wireguard"
)

func BGPFlagSet() (*flag.FlagSet, func() BGPConfig) {
	fs := flag.NewFlagSet("net/bgp", flag.ExitOnError)

	binary := fs.String("net-bgp-bird-binary", "bird", "the path to the bird daemon binary")
	path := fs.String("net-bgp-path", "/var/lib/teapot/net/bgp", "the path to store the bgp config and control socket")
	device := fs.String("net-bgp-device", wireguard.DefaultWireguardDevice, "the device on which to perform BGP. It should be the same as the wireguard device")

	return fs, func() BGPConfig {
		return BGPConfig{
			Binary: *binary,
			Path:   *path,
			Device: *device,
		}
	}
}
