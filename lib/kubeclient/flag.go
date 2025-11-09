package kubeclient

import (
	flag "github.com/spf13/pflag"
)

func KubeClientFlagSet() (*flag.FlagSet, func() KubeClientConfig) {
	fs := flag.NewFlagSet("kubeclient", flag.ExitOnError)

	kubeConfig := fs.String("kubeconfig", "", "the path to the kubeconfig file")

	return fs, func() KubeClientConfig {
		return KubeClientConfig{
			KubeConfig: *kubeConfig,
		}
	}
}
