package loadbalancer

import (
	"context"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Reconcile is triggered for a Service. It finds all pods matching the service
// selector, collects the external IPs of their nodes, and sets them on the
// Service.Status.LoadBalancer.Ingress.
func (lb *LoadBalancer) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	svc := &v1.Service{}
	if err := lb.client.Get(ctx, req.NamespacedName, svc); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	if svc.Spec.Type != v1.ServiceTypeLoadBalancer {
		return reconcile.Result{}, nil
	}

	// List pods matching the service selector.
	podList := &v1.PodList{}
	if err := lb.client.List(ctx, podList,
		client.InNamespace(svc.Namespace),
		client.MatchingLabels(svc.Spec.Selector),
	); err != nil {
		return reconcile.Result{}, err
	}

	// Collect unique node names from running pods.
	nodeNames := map[string]struct{}{}
	for _, pod := range podList.Items {
		if pod.Spec.NodeName != "" && pod.Status.Phase == v1.PodRunning {
			nodeNames[pod.Spec.NodeName] = struct{}{}
		}
	}

	// Collect external IPs from those nodes.
	seen := map[string]struct{}{}
	var ingresses []v1.LoadBalancerIngress
	for nodeName := range nodeNames {
		node := &v1.Node{}
		if err := lb.client.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
			lb.logger.Error("failed to get node", "node", nodeName, "error", err)
			continue
		}
		for _, addr := range node.Status.Addresses {
			if addr.Type == v1.NodeExternalIP {
				if _, ok := seen[addr.Address]; !ok {
					seen[addr.Address] = struct{}{}
					ingresses = append(ingresses, v1.LoadBalancerIngress{IP: addr.Address})
				}
			}
		}
	}

	// Patch status only when the ingress list has changed.
	patch := client.MergeFrom(svc.DeepCopy())
	svc.Status.LoadBalancer.Ingress = ingresses
	if err := lb.client.Status().Patch(ctx, svc, patch); err != nil {
		return reconcile.Result{}, err
	}

	lb.logger.Info("updated service ingress IPs", "service", req.NamespacedName, "ips", seen)
	return reconcile.Result{}, nil
}

// podToService maps a Pod event to the Services that select it.
func (lb *LoadBalancer) podToService(ctx context.Context, obj client.Object) []reconcile.Request {
	pod := obj.(*v1.Pod)

	svcList := &v1.ServiceList{}
	if err := lb.client.List(ctx, svcList, client.InNamespace(pod.Namespace)); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, svc := range svcList.Items {
		if svc.Spec.Type != v1.ServiceTypeLoadBalancer || len(svc.Spec.Selector) == 0 {
			continue
		}
		if labelsMatch(svc.Spec.Selector, pod.Labels) {
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

func labelsMatch(selector, labels map[string]string) bool {
	for k, v := range selector {
		if labels[k] != v {
			return false
		}
	}
	return true
}
