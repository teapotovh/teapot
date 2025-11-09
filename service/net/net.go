package net

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/teapotovh/teapot/lib/kubeclient"
	"github.com/teapotovh/teapot/lib/kubecontroller"
)

var (
	ErrMissingNode = errors.New("no node name provided, net cannot tell which node it's running on")
)

type NetConfig struct {
	KubeClientConfig kubeclient.KubeClientConfig
	Node string
}

type Net struct {
	logger *slog.Logger

	client *kubernetes.Clientset
	controller *kubecontroller.Controller[*v1.Node]
}

func NewNet(config NetConfig, logger *slog.Logger) (*Net, error) {
	if config.Node == "" {
		return nil, ErrMissingNode
	}

	client, err := kubeclient.NewKubeClient(config.KubeClientConfig, logger.With("component", "kubeclient"))
	if err != nil {
		return nil, fmt.Errorf("error while building kubernetes client: %w", err)
	}

	net := Net{
		logger: logger,
		client: client,
	}

	controllerConfig := kubecontroller.ControllerConfig[*v1.Node]{
		Client: client,
		Handler: net.handle,
	}
	net.controller, err = kubecontroller.NewController(controllerConfig, logger.With("component", "kubecontroller"))
	if err != nil {
		return nil, fmt.Errorf("error while building kubernetes controller: %w", err)
	}

	return &net, nil
}

func (net *Net) handle(node *v1.Node, exists bool) error {
	if !exists {
			net.logger.Debug("node was removed")
	}

	external, ok := node.Annotations["net.teapot.ovh/external-ip"]
		net.logger.Info("got node", "external", external, "ok", ok)

	return nil
}

func (net *Net) Run(ctx context.Context) error {
	return net.controller.Run(ctx, 1)
}
