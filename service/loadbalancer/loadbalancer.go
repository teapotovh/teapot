package loadbalancer

import (
	"context"
	"fmt"
	"log/slog"
	"net/netip"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/teapotovh/teapot/lib/broker"
	"github.com/teapotovh/teapot/lib/kubeclient"
	"github.com/teapotovh/teapot/lib/kubecontroller"
	"github.com/teapotovh/teapot/lib/run"
)

type LoadBalancerConfig struct {
	KubeClient kubeclient.KubeClientConfig
}

type LoadBalancer struct {
	logger *slog.Logger

	client       *kubernetes.Clientset
	broker       *broker.Broker[Event]
	brokerCancel context.CancelFunc

	controller *kubecontroller.Controller[*v1.Service]
	state      map[string][]netip.Addr
	prevEvent  Event
}

func NewLoadBalancer(config LoadBalancerConfig, logger *slog.Logger) (*LoadBalancer, error) {
	client, err := kubeclient.NewKubeClient(config.KubeClient, logger.With("component", "kubeclient"))
	if err != nil {
		return nil, fmt.Errorf("error while building kubernetes client: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	lb := LoadBalancer{
		logger:       logger,
		client:       client,
		broker:       broker.NewBroker[Event](),
		brokerCancel: cancel,
		state:        map[string][]netip.Addr{},
	}

	controllerConfig := kubecontroller.ControllerConfig[*v1.Service]{
		Client:  client,
		Handler: lb.handle,
	}
	lb.controller, err = kubecontroller.NewController(controllerConfig, logger.With("component", "kubecontroller"))
	if err != nil {
		return nil, fmt.Errorf("error while building kubernetes controller: %w", err)
	}

	go lb.broker.Run(ctx)
	return &lb, nil
}

func (lb *LoadBalancer) Broker() *broker.Broker[Event] {
	return lb.broker
}

// Run implements run.Runnable
func (lb *LoadBalancer) Run(ctx context.Context, notify run.Notify) error {
	defer lb.brokerCancel()

	notify.Notify()
	return lb.controller.Run(ctx, 1)
}
