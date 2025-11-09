package net

import (
	"context"
	"fmt"
	"log/slog"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/teapotovh/teapot/lib/kubeclient"
	"github.com/teapotovh/teapot/lib/kubecontroller"
)

type NetConfig struct {
	KubeClientConfig kubeclient.KubeClientConfig
}

type Net struct {
	logger *slog.Logger

	client *kubernetes.Clientset
	controller *kubecontroller.Controller[*v1.Node]
}

func NewNet(config NetConfig, logger *slog.Logger) (*Net, error) {
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

func (net *Net) handle(resource *v1.Node, exists bool) error {
	net.logger.Info("got resource", "resource", resource, "exists", exists)
	return nil
}

func (net *Net) Run(ctx context.Context) error {
	return net.controller.Run(ctx, 1)
}
