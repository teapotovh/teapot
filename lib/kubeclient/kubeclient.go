package kubeclient

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type KubeClientConfig struct {
	KubeConfig string
}

func NewKubeClient(config KubeClientConfig, logger *slog.Logger) (*kubernetes.Clientset, error) {
	cfg, err := getConfig(config.KubeConfig, logger)
	if err != nil {
		return nil, fmt.Errorf("error while loading kubernetes config: %w", err)
	}

	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("error while building kubernetes client: %w", err)
	}

	return client, nil
}

func getConfig(kubeconfig string, logger *slog.Logger) (*rest.Config, error) {
	var path *string
	if kubeconfig == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			err = fmt.Errorf("error while getting user home directory: %w", err)
			logger.Warn("cannot derive kubernetes default config path", "err", err)
		}

		p := filepath.Join(home, ".kube", "config")
		_, err = os.Stat(p)
		if err == nil {
			path = &p
		} else {
			if !errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("unexpected error while checking if kubernetes config exists: %w", err)
			}
		}
	} else {
		path = &kubeconfig
	}

	if path == nil {
		logger.Warn("no kubeconfig path provided, falling back to in-cluster config")
		cfg, err := rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("error while getting kubernetes in-cluster config: %w", err)
		}

		return cfg, nil
	} else {
		config, err := os.ReadFile(*path)
		if err != nil {
			return nil, fmt.Errorf("error while reading kubernetes config file at %s: %w", *path, err)
		}

		cfg, err := clientcmd.RESTConfigFromKubeConfig(config)
		if err != nil {
			return nil, fmt.Errorf("error while parsing kubernetes config: %w", err)
		}

		return cfg, nil
	}
}
