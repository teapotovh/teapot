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
	"github.com/teapotovh/teapot/lib/run"
)

var ErrMissingNode = errors.New("no node name provided, net cannot tell which node it's running on")

type NetConfig struct {
	KubeClient kubeclient.KubeClientConfig
	Node       string
	Local      LocalConfig
	Cluster    ClusterConfig
}

type Net struct {
	logger *slog.Logger

	client       *kubernetes.Clientset
	broker       *broker.Broker[Event]
	brokerCancel context.CancelFunc

	controller *kubecontroller.Controller[*v1.Node]
	local      *Local
	cluster    *Cluster
}

func NewNet(config NetConfig, logger *slog.Logger) (*Net, error) {
	if config.Node == "" {
		return nil, ErrMissingNode
	}

	client, err := kubeclient.NewKubeClient(config.KubeClient, logger.With("component", "kubeclient"))
	if err != nil {
		return nil, fmt.Errorf("error while building kubernetes client: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	net := Net{
		logger:       logger,
		client:       client,
		broker:       broker.NewBroker[Event](),
		brokerCancel: cancel,
	}

	net.local, err = NewLocal(&net, config.Local, logger.With("component", "local"))
	if err != nil {
		return nil, fmt.Errorf("error while building net's local component: %w", err)
	}

	net.cluster, err = NewCluster(&net, config.Cluster, logger.With("component", "cluster"))
	if err != nil {
		return nil, fmt.Errorf("error while building net's cluster component: %w", err)
	}

	controllerConfig := kubecontroller.ControllerConfig[*v1.Node]{
		Client:  client,
		Handler: net.handle,
	}

	net.controller, err = kubecontroller.NewController(controllerConfig, logger.With("component", "kubecontroller"))
	if err != nil {
		return nil, fmt.Errorf("error while building kubernetes controller: %w", err)
	}

	go net.broker.Run(ctx)

	return &net, nil
}

func (net *Net) Client() *kubernetes.Clientset {
	return net.client
}

func (net *Net) Local() *Local {
	return net.local
}

func (net *Net) Cluster() *Cluster {
	return net.cluster
}

// Run implements run.Runnable.
func (net *Net) Run(ctx context.Context, notify run.Notify) error {
	defer net.brokerCancel()

	notify.Notify()

	return net.controller.Run(ctx, 1)
}
