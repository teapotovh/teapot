package checkblocklist

import (
	"strings"

	flag "github.com/spf13/pflag"
	"github.com/teapotovh/teapot/lib/kubeclient"
)

var allBlockLists []string

func init() {
	for _, bln := range AllBlockListNames {
		allBlockLists = append(allBlockLists, string(bln))
	}
}

func CheckBlockListFlagSet() (*flag.FlagSet, func() CheckBlockListConfig) {
	fs := flag.NewFlagSet("checkblocklist", flag.ExitOnError)

	lists := fs.StringArray(
		"checkblocklist-lists",
		[]string{},
		"a list of blocklists to check. Available options: "+strings.Join(allBlockLists, ","),
	)

	maxRetries := fs.Uint64(
		"checkblocklist-max-retries",
		4,
		"maximum number of retries for DNS queries",
	)

	kubeCilentFS, getKubeCilentConfig := kubeclient.KubeClientFlagSet()
	fs.AddFlagSet(kubeCilentFS)

	return fs, func() CheckBlockListConfig {
		return CheckBlockListConfig{
			KubeClient: getKubeCilentConfig(),
			Lists:      *lists,
			MaxRetries: *maxRetries,
		}
	}
}
