package checkblocklist

import (
	"fmt"
	"net/netip"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/teapotovh/teapot/lib/kubeclient"
	"github.com/teapotovh/teapot/lib/kubecontroller"
)

type CheckBlockListController struct {
	client     *kubernetes.Clientset
	controller *kubecontroller.Controller[*v1.Node]
}

type CheckBlockListControllerConfig struct {
	KubeClient kubeclient.KubeClientConfig
}

func NewCheckBlockListController(config CheckBlockListConfig, logger *slog.Logger) (*CheckBlockList, error) {
}

func (cblc *CheckBlockListController) handle(name string, n *v1.Node, exists bool) error {
	if _, previouslyExisted := cblc.ips[name]; !exists && previouslyExisted {
		cblc.logger.Info("node was removed, removing externalIP from watch list", "name", name, "externalIP", cblc.ips[name])
		delete(cblc.ips, name)
		return nil
	}

	ip := netip.IPv4Unspecified()
	for _, addr := range n.Status.Addresses {
		switch addr.Type {
		case v1.NodeExternalIP:
			var err error
			ip, err = netip.ParseAddr(addr.Address)
			if err != nil {
				return fmt.Errorf("error while parsing the external ip %q for node %s: %w", addr.Address, n.Name, err)
			}
		case v1.NodeHostName, v1.NodeInternalIP, v1.NodeInternalDNS, v1.NodeExternalDNS:
			continue
		}
	}

	if !ip.IsUnspecified() {
	}

	cblc.logger.Info("got update", "name", name, "node", n, "exists", exists)
	return nil
}

// Run implements run.Runnable.
func (cblc *CheckBlockListController) Run(ctx context.Context, notify run.Notify) error {
	notify.Notify()

	return cblc.controller.Run(ctx, 1)
}
