package net

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/teapotovh/teapot/lib/broker"
	"github.com/teapotovh/teapot/lib/kubeclient"
	"github.com/teapotovh/teapot/lib/kubecontroller"
)

var (
	ErrMissingNode = errors.New("no node name provided, net cannot tell which node it's running on")
)

type NetConfig struct {
	KubeClientConfig kubeclient.KubeClientConfig
	Node string
	Local LocalConfig
	Wireguard WireguardConfig
}

type Net struct {
	logger *slog.Logger

	client *kubernetes.Clientset
	broker *broker.Broker[Event]

	controller *kubecontroller.Controller[*v1.Node]
	local *Local
	wireguard *Wireguard
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

	net.local, err = NewLocal(&net, config.Local, logger.With("component", "local"))
	if err != nil {
		return nil, fmt.Errorf("error while building net's local component: %w", err)
	}

	net.wireguard, err = NewWireguard(&net, config.Wireguard, logger.With("component", "wireguard"))
	if err != nil {
		return nil, fmt.Errorf("error while building net's wireguard component: %w", err)
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

func (net *Net) Client() *kubernetes.Clientset {
	return net.client
}

func (net *Net) Broker() *broker.Broker[Event] {
	return net.broker
}

func (net *Net) Local() *Local {
	return net.local
}

func (net *Net) Wireguard() *Wireguard {
	return net.wireguard
}

func (net *Net) Run(ctx context.Context) error {
	net.broker = broker.NewBroker[Event]()
	go net.broker.Run(ctx)

	go net.local.Run(ctx)
	go net.wireguard.Run(ctx)

	return net.controller.Run(ctx, 1)
}
