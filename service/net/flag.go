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

	return fs, func() NetConfig {
		return NetConfig{
			KubeClientConfig: getKubeCilentConfig(),
			Node:             *node,
		}
	}
}
