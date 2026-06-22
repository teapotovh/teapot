package loadbalancer

import (
	"context"
	"fmt"
	"log/slog"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/teapotovh/teapot/lib/kubeclient"
	"github.com/teapotovh/teapot/lib/run"
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
	cfg, err := kubeclient.GetConfig(config.KubeClient, logger.With("component", "kubeclient"))
	if err != nil {
		return nil, fmt.Errorf("error while getting kubeconfig: %w", err)
	}

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Metrics: metricsserver.Options{
			BindAddress: "0",
		},
	})
	if err != nil {
		return nil, fmt.Errorf("error while creating controller manager: %w", err)
	}

	r := &LoadBalancer{
		logger: logger,
		client: mgr.GetClient(),
		mgr:    mgr,
	}

	// Watch Services with a LoadBalancer type predicate, and Pods that map back
	// to Services via the podToService handler.
	err = ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Service{}, builder.WithPredicates(predicate.NewPredicateFuncs(func(obj client.Object) bool {
			svc, ok := obj.(*corev1.Service)
			return ok && svc.Spec.Type == corev1.ServiceTypeLoadBalancer
		}))).
		Watches(&corev1.Pod{}, handler.EnqueueRequestsFromMapFunc(r.podToService)).
		Watches(&corev1.Node{}, handler.EnqueueRequestsFromMapFunc(r.nodeToService)).
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

// podToService maps a Pod event to the Services that select it.
func (lb *LoadBalancer) podToService(ctx context.Context, obj client.Object) []reconcile.Request {
	pod := obj.(*corev1.Pod)

	svcList := &corev1.ServiceList{}
	if err := lb.client.List(ctx, svcList, client.InNamespace(pod.Namespace)); err != nil {
		return nil
	}

	var requests []reconcile.Request

	for _, svc := range svcList.Items {
		if svc.Spec.Type != corev1.ServiceTypeLoadBalancer {
			continue
		}

		selector := labels.SelectorFromSet(svc.Spec.Selector)

		if selector.Matches(labels.Set(pod.Labels)) {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: svc.Namespace,
					Name:      svc.Name,
				},
			})
		}
	}

	return requests
}

// nodeToService maps a Node event to the Pods that are on it and then to the
// Services that select them.
func (lb *LoadBalancer) nodeToService(ctx context.Context, obj client.Object) []reconcile.Request {
	node := obj.(*corev1.Node)

	podList := &corev1.PodList{}
	if err := lb.client.List(ctx, podList, client.MatchingFields{"spec.nodeName": node.Name}); err != nil {
		return nil
	}

	seen := map[types.NamespacedName]struct{}{}

	var requests []reconcile.Request

	for _, pod := range podList.Items {
		for _, req := range lb.podToService(ctx, &pod) {
			if _, ok := seen[req.NamespacedName]; !ok {
				seen[req.NamespacedName] = struct{}{}
				requests = append(requests, req)
			}
		}
	}

	return requests
}
