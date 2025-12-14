package loadbalancer

import (
	flag "github.com/spf13/pflag"

	"github.com/teapotovh/teapot/lib/kubeclient"
)

func LoadBalancerFlagSet() (*flag.FlagSet, func() LoadBalancerConfig) {
	fs := flag.NewFlagSet("loadbalancer", flag.ExitOnError)

	kubeCilentFS, getKubeCilentConfig := kubeclient.KubeClientFlagSet()
	fs.AddFlagSet(kubeCilentFS)

	return fs, func() LoadBalancerConfig {
		return LoadBalancerConfig{
			KubeClient: getKubeCilentConfig(),
		}
	}
}
