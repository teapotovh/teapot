package kubeutil

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
)

func AnnotateNode(
	ctx context.Context,
	clientset *kubernetes.Clientset,

	name string,
	key string,
	value string,
) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		node, err := clientset.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get kubernetes node %q: %w", name, err)
		}

		if node.Annotations == nil {
			node.Annotations = make(map[string]string)
		}

		node.Annotations[key] = value

		if _, err := clientset.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("failed to update kubernetes node %q: %w", node.Name, err)
		}

		return nil
	})
}
