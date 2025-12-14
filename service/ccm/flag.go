package ccm

import (
	flag "github.com/spf13/pflag"

	"github.com/teapotovh/teapot/lib/kubeclient"
)

func CCMFlagSet() (*flag.FlagSet, func() CCMConfig) {
	fs := flag.NewFlagSet("ccm", flag.ExitOnError)

	node := fs.String("ccm-node", "", "the name of the kubernetes node where ccmd is running on")

	kubeCilentFS, getKubeCilentConfig := kubeclient.KubeClientFlagSet()
	fs.AddFlagSet(kubeCilentFS)

	return fs, func() CCMConfig {
		return CCMConfig{
			Node:       *node,
			KubeClient: getKubeCilentConfig(),
		}
	}
}
