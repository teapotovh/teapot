package kubeutil

import (
	"context"
	"fmt"
	"slices"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func RemoveTaint(ctx context.Context, clientset *kubernetes.Clientset, nodeName, key, value string) error {
	node, err := clientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get kubernetes node %q: %w", nodeName, err)
	}

	index := slices.IndexFunc(node.Spec.Taints, func(taint corev1.Taint) bool {
		return taint.Key == key && taint.Value == value
	})
	if index == -1 {
		return nil
	}

	node.Spec.Taints = slices.Delete(node.Spec.Taints, index, index+1)

	_, err = clientset.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update taint %q kubernetes node %q: %w", key, node.Name, err)
	}

	return nil
}
