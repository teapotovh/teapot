package kubelog

import (
	"log/slog"

	"github.com/go-logr/logr"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func WithLogger(logger *slog.Logger) {
	klog.SetSlogLogger(logger)
	log.SetLogger(logr.FromSlogHandler(logger.Handler()))
}
