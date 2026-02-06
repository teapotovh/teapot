package checkblocklist

import (
	"context"
	"fmt"
	"log/slog"
	"net/netip"

	"github.com/teapotovh/teapot/lib/kubeclient"
	"github.com/teapotovh/teapot/lib/kubecontroller"
	"github.com/teapotovh/teapot/lib/run"
)

type CheckBlockList struct {
	logger *slog.Logger

	lists   map[BlockListName]BlockList
	ips     map[string]netip.Addr
	metrics metrics
}

type CheckBlockListConfig struct {
	KubeClient kubeclient.KubeClientConfig
	Lists      []string
	MaxRetries uint64
}

func NewCheckBlockList(config CheckBlockListConfig, logger *slog.Logger) (*CheckBlockList, error) {
	names, err := ParseBlockListNames(config.Lists)
	if err != nil {
		return nil, fmt.Errorf("error while parsing names for configured block lists: %w", err)
	}
	lists := map[BlockListName]BlockList{}
	for _, name := range names {
		lists[name] = Lists[name]
	}

	client, err := kubeclient.NewKubeClient(config.KubeClient, logger.With("component", "kubeclient"))
	if err != nil {
		return nil, fmt.Errorf("error while building kubernetes client: %w", err)
	}

	cbl := CheckBlockList{
		logger: logger,

		lists: lists,

		client: client,
	}

	controllerConfig := kubecontroller.ControllerConfig[*v1.Node]{
		Client:  client,
		Handler: cbl.handle,
	}

	cbl.controller, err = kubecontroller.NewController(controllerConfig, logger.With("component", "kubecontroller"))
	if err != nil {
		return nil, fmt.Errorf("error while building kubernetes controller: %w", err)
	}

	cbl.initMetrics()

	return &cbl, nil
}

// Run implements run.Runnable.
func (cbl *CheckBlockList) Run(ctx context.Context, notify run.Notify) error {
	notify.Notify()

	return cbl.controller.Run(ctx, 1)
}
