package net

import (
	flag "github.com/spf13/pflag"
	"github.com/teapotovh/teapot/lib/kubeclient"
)

func NetFlagSet() (*flag.FlagSet, func() NetConfig) {
	fs := flag.NewFlagSet("net", flag.ExitOnError)

	node := fs.String("net-node", "", "the name of the kubernetes node where netd is running on")

	kubeCilentFS, getKubeCilentConfig := kubeclient.KubeClientFlagSet()
	fs.AddFlagSet(kubeCilentFS)

	localFS, getLocalConfig := LocalFlagSet()
	fs.AddFlagSet(localFS)

	wireguardFS, getWireguardConfig := WireguardFlagSet()
	fs.AddFlagSet(wireguardFS)

	return fs, func() NetConfig {
		local := getLocalConfig()
		local.LocalNode = *node

		wireguard := getWireguardConfig()
		wireguard.LocalNode = *node

		return NetConfig{
			KubeClientConfig: getKubeCilentConfig(),
			Node:             *node,
			Local:            local,
			Wireguard:        wireguard,
		}
	}
}
