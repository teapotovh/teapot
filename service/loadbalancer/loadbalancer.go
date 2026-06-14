package loadbalancer

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/teapotovh/teapot/lib/kubeclient"
	"github.com/teapotovh/teapot/lib/run"
	v1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

type LoadBalancerConfig struct {
	KubeClient kubeclient.KubeClientConfig
}

type LoadBalancer struct {
	logger *slog.Logger

	mgr    ctrl.Manager
	client client.Client
}

func NewLoadBalancer(config LoadBalancerConfig, logger *slog.Logger) (*LoadBalancer, error) {
	// ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
	cfg, err := kubeclient.GetConfig(config.KubeClient, logger.With("component", "kubeclient"))
	if err != nil {
		return nil, fmt.Errorf("error while getting kubeconfig: %w", err)
	}

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{})
	if err != nil {
		return nil, fmt.Errorf("error while creating controller manager: %w", err)
	}

	r := &LoadBalancer{
		logger: logger,
		client: mgr.GetClient(),
	}

	// Watch Services with a LoadBalancer type predicate, and Pods that map back
	// to Services via the podToService handler.
	err = ctrl.NewControllerManagedBy(mgr).
		For(&v1.Service{}, builder.WithPredicates(predicate.NewPredicateFuncs(func(obj client.Object) bool {
			svc, ok := obj.(*v1.Service)
			return ok && svc.Spec.Type == v1.ServiceTypeLoadBalancer
		}))).
		Watches(&v1.Pod{}, handler.EnqueueRequestsFromMapFunc(r.podToService)).
		Complete(r)
	if err != nil {
		return nil, fmt.Errorf("error while setting up controller: %w", err)
	}

	return r, nil
}

// Run implements run.Runnable.
func (lb *LoadBalancer) Run(ctx context.Context, notify run.Notify) error {
	notify.Notify()

	return lb.mgr.Start(ctx)
}
