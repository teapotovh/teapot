package ccm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"sync"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"

	"github.com/teapotovh/teapot/lib/broker"
	"github.com/teapotovh/teapot/lib/kubeclient"
	"github.com/teapotovh/teapot/lib/kubecontroller"
	"github.com/teapotovh/teapot/lib/run"
)

var ErrMissingNode = errors.New("no node name provided, ccm cannot tell which node it's running on")

type CCMConfig struct {
	Node       string
	KubeClient kubeclient.KubeClientConfig
}

type CCM struct {
	internalIP   netip.Addr
	externalIP   netip.Addr
	logger       *slog.Logger
	client       *kubernetes.Clientset
	broker       *broker.Broker[Event]
	brokerCancel context.CancelFunc
	controller   *kubecontroller.Controller[*v1.Node]
	node         string
	lock         sync.Mutex
}

func NewCCM(config CCMConfig, logger *slog.Logger) (*CCM, error) {
	if config.Node == "" {
		return nil, ErrMissingNode
	}

	client, err := kubeclient.NewKubeClient(config.KubeClient, logger.With("component", "kubeclient"))
	if err != nil {
		return nil, fmt.Errorf("error while building kubernetes client: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	ccm := CCM{
		logger: logger,

		node:         config.Node,
		client:       client,
		broker:       broker.NewBroker[Event](),
		brokerCancel: cancel,
	}
	controllerConfig := kubecontroller.ControllerConfig[*v1.Node]{
		Client:  client,
		Handler: ccm.handle,
	}
	ccm.controller, err = kubecontroller.NewController(controllerConfig, logger.With("component", "kubecontroller"))
	if err != nil {
		return nil, fmt.Errorf("error while building kubernetes controller: %w", err)
	}

	go ccm.broker.Run(ctx)
	return &ccm, nil
}

type Event struct {
	Node       string
	ExternalIP netip.Addr
	InternalIP netip.Addr
	Hostname   string
}

func (ccm *CCM) handle(name string, node *v1.Node, exists bool) error {
	if name != ccm.node {
		return nil
	}

	var (
		externalIP, internalIP netip.Addr
		hostname               string
		err                    error
	)

	for _, addr := range node.Status.Addresses {
		// NOTE: we don't want to return an error here if address parsing fails.
		// That's because, we want this controller to overwrite malicious updates,
		// which may also set these fields to invalid addresses.
		switch addr.Type {
		case v1.NodeExternalIP:
			externalIP, err = netip.ParseAddr(addr.Address)
			if err != nil {
				err = fmt.Errorf("error while parsing ExternalIP: %w", err)
				ccm.logger.Warn("could not parse Status.Addresses.ExternalIP", "err", err)
			}
		case v1.NodeInternalIP:
			internalIP, err = netip.ParseAddr(addr.Address)
			if err != nil {
				err = fmt.Errorf("error while parsing InternalIP: %w", err)
				ccm.logger.Warn("could not parse Status.Addresses.InternalIP", "err", err)
			}
		case v1.NodeHostName:
			// TODO: we should figure out a component that reconciliates the Hostname
			// to something that can be resolved by nodes. Possibly, we also want to
			// have an internal DNS name.
			hostname = addr.Address
		}
	}

	ccm.broker.Publish(Event{
		Node:       node.Name,
		ExternalIP: externalIP,
		InternalIP: internalIP,
		Hostname:   hostname,
	})
	return nil
}

func (ccm *CCM) Broker() *broker.Broker[Event] {
	return ccm.broker
}

func (ccm *CCM) SetInternalIP(ctx context.Context, addr netip.Addr) error {
	ccm.internalIP = addr
	return ccm.update(ctx)
}

func (ccm *CCM) SetExternalIP(ctx context.Context, addr netip.Addr) error {
	ccm.externalIP = addr
	return ccm.update(ctx)
}

func (ccm *CCM) update(ctx context.Context) error {
	ccm.lock.Lock()
	defer ccm.lock.Unlock()

	addresses := []v1.NodeAddress{
		{
			Type:    v1.NodeHostName,
			Address: ccm.node,
		},
	}

	if ccm.internalIP.IsValid() {
		addresses = append(addresses, v1.NodeAddress{
			Type:    v1.NodeInternalIP,
			Address: ccm.internalIP.String(),
		})
	}

	if ccm.externalIP.IsValid() {
		addresses = append(addresses, v1.NodeAddress{
			Type:    v1.NodeExternalIP,
			Address: ccm.externalIP.String(),
		})
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		node, err := ccm.client.CoreV1().Nodes().Get(ctx, ccm.node, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get kubernetes node %q: %w", ccm.node, err)
		}

		node.Status.Addresses = addresses

		if _, err := ccm.client.CoreV1().Nodes().UpdateStatus(ctx, node, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("failed to update kubernetes node %q: %w", node.Name, err)
		}

		return nil
	})
}

func (ccm *CCM) KubeClient() *kubernetes.Clientset {
	return ccm.client
}

// Run implements run.Runnable
func (ccm *CCM) Run(ctx context.Context, notify run.Notify) error {
	defer ccm.brokerCancel()

	notify.Notify()
	return ccm.controller.Run(ctx, 1)
}
