package net

import (
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

	client *kubernetes.Clientset
	broker *broker.Broker[Event]

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

	net := Net{
		logger: logger,
		client: client,
		broker: broker.NewBroker[Event](),
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
		Workers: 1,
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

func (net *Net) Local() *Local {
	return net.local
}

func (net *Net) Cluster() *Cluster {
	return net.cluster
}

// Run implements run.Runnable.
func (net *Net) Run(step run.Step) error {
	step.Add("broker", net.broker)
	step.Add("controller", net.controller)
	step.Add("local", net.local)
	step.Add("cluster", net.cluster)
	step.Wait()
	step.Notify()

	<-step.Context().Done()

	return nil
}
