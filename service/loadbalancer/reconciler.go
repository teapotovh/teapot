package loadbalancer

import (
	"context"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type unit struct{}

func ptr[T any](value T) *T {
	return new(value)
}

// Reconcile is triggered for a Service. It finds all pods matching the service
// selector, collects the external IPs of their nodes, and sets them on the
// Service.Status.LoadBalancer.Ingress.
func (lb *LoadBalancer) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	svc := &corev1.Service{}
	if err := lb.client.Get(ctx, req.NamespacedName, svc); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	if svc.Spec.Type != corev1.ServiceTypeLoadBalancer {
		return reconcile.Result{}, nil
	}

	// List pods matching the service selector.
	podList := &corev1.PodList{}
	if err := lb.client.List(ctx, podList,
		client.InNamespace(svc.Namespace),
		client.MatchingLabels(svc.Spec.Selector),
	); err != nil {
		return reconcile.Result{}, err
	}

	// Collect unique node names from running pods.
	nodeNames := map[string]unit{}

	for _, pod := range podList.Items {
		if pod.Spec.NodeName != "" && pod.Status.Phase == corev1.PodRunning {
			nodeNames[pod.Spec.NodeName] = unit{}
		}
	}

	// Collect external IPs from those nodes.
	seen := map[string]unit{}

	var ingresses []corev1.LoadBalancerIngress

	for nodeName := range nodeNames {
		node := &corev1.Node{}
		if err := lb.client.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
			lb.logger.Error("failed to get node", "node", nodeName, "error", err)
			continue
		}

		if !nodeIsReady(node) {
			lb.logger.Warn("skipping node as it's not ready", "node", node.Name, "conditions", node.Status.Conditions)
			continue
		}

		for _, addr := range node.Status.Addresses {
			if addr.Type == corev1.NodeExternalIP {
				if _, ok := seen[addr.Address]; !ok {
					seen[addr.Address] = unit{}
					ingresses = append(ingresses, corev1.LoadBalancerIngress{
						IP:     addr.Address,
						IPMode: ptr(corev1.LoadBalancerIPModeProxy),
					})
				}
			}
		}
	}

	// Patch status only when the ingress list has changed.
	if !reflect.DeepEqual(svc.Status.LoadBalancer.Ingress, ingresses) {
		patch := client.MergeFrom(svc.DeepCopy())
		svc.Status.LoadBalancer.Ingress = ingresses

		if err := lb.client.Status().Patch(ctx, svc, patch); err != nil {
			return reconcile.Result{}, err
		}

		lb.logger.Info("updated LoadBalancer service's ingress IPs", "service", req.NamespacedName, "ips", seen)
	}

	return reconcile.Result{}, nil
}

func nodeIsReady(node *corev1.Node) bool {
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady {
			return cond.Status == corev1.ConditionTrue
		}
	}

	return false
}
