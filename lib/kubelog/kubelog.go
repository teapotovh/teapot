package kubelog

import (
	"log/slog"

	"k8s.io/klog/v2"
)

func WithLogger(logger *slog.Logger) {
	klog.SetSlogLogger(logger.With("component", "klog"))
}
