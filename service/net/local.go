package net

import (
	"context"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"path/filepath"

	"github.com/teapotovh/teapot/lib/broker"
	"github.com/teapotovh/teapot/lib/run"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

const (
	KeyFilename = "wireguard.key"
	KeyPerm     = os.FileMode(0660)
)

type Local struct {
	logger *slog.Logger
	net    *Net

	node  string
	key   wgtypes.Key
	cirds []netip.Prefix

	broker       *broker.Broker[ClusterEvent]
	brokerCancel context.CancelFunc
}

type LocalConfig struct {
	LocalNode string
	Path      string
}

func NewLocal(net *Net, config LocalConfig, logger *slog.Logger) (*Local, error) {
	path := filepath.Clean(config.Path)
	if err := os.MkdirAll(path, os.ModePerm); err != nil {
		return nil, fmt.Errorf("error while ensuring local state directory exists: %w", err)
	}

	keyPath := filepath.Join(path, KeyFilename)
	key, err := getKey(keyPath)
	if err != nil {
		logger.Warn("error while loading previous wireguard key, generating a new one", "err", err)

		key, err = wgtypes.GeneratePrivateKey()
		if err != nil {
			return nil, fmt.Errorf("error while generating wireguard key for local node: %w", err)
		}

		if err := storeKey(keyPath, key); err != nil {
			return nil, fmt.Errorf("error while generating wireguard key for local node: %w", err)
		}
	}
	logger.Info("loaded wireguard key", "publicKey", key.PublicKey())

	ctx, cancel := context.WithCancel(context.Background())
	broker := broker.NewBroker[ClusterEvent]()
	go broker.Run(ctx)

	return &Local{
		logger: logger,
		net:    net,

		node: config.LocalNode,
		key:  key,

		broker:       broker,
		brokerCancel: cancel,
	}, nil
}

func getKey(path string) (wgtypes.Key, error) {
	encodedKey, err := os.ReadFile(path)
	if err != nil {
		return wgtypes.Key{}, fmt.Errorf("error while reading key file from filesystem: %w", err)
	}

	key, err := wgtypes.ParseKey(string(encodedKey))
	if err != nil {
		return wgtypes.Key{}, fmt.Errorf("error while parsing private key: %w", err)
	}

	return key, nil
}

func storeKey(path string, key wgtypes.Key) error {
	encodedKey := key.String()
	err := os.WriteFile(path, []byte(encodedKey), KeyPerm)
	if err != nil {
		return fmt.Errorf("error while writing wireguard key to filesystem: %w", err)
	}

	return nil
}

// Run implements run.Runnable
func (l *Local) Run(ctx context.Context, notify run.Notify) error {
	defer l.brokerCancel()
	sub := l.net.broker.Subscribe()
	defer sub.Unsubscribe()

	notify.Notify()
	for event := range sub.Iter(ctx) {
		if event.Update != nil {
			node := *event.Update

			if node.Name == l.node {
				l.logger.Debug("received update", "node", node)

				pk := l.key.PublicKey()
				if node.PublicKey == nil || *node.PublicKey != pk {
					l.logger.Warn("kubernetes wireguard key differs from local, updating", "node", node.Name)

					if err := annotateNode(ctx, l.net.Client(), node.Name, AnnotationPublicKey, pk.String()); err != nil {
						return fmt.Errorf("error while storing public key in node %q annotation: %w", node.Name, err)
					} else {
						l.logger.Info("updated public key", "node", node.Name, "publicKey", pk)
					}
				}
			}
		}
	}

	return nil
}

func (l *Local) PrivateKey() wgtypes.Key {
	return l.key
}

func (l *Local) PublicKey() wgtypes.Key {
	return l.key.PublicKey()
}

func (l *Local) CIDRs() []netip.Prefix {
	return l.cirds
}

func (l *Local) Node() string {
	return l.node
}
