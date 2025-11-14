package bgp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"

	"github.com/teapotovh/teapot/lib/cmd"
	"github.com/teapotovh/teapot/lib/run"
	tnet "github.com/teapotovh/teapot/service/net"
)

const (
	ConfigPerm = os.FileMode(0644)
)

var (
	ErrBirdNotRunning = errors.New("bird process not running")
)

type BGP struct {
	logger *slog.Logger
	net    *tnet.Net

	cmd        *cmd.Command
	config     string
	binary     string
	configPath string
	socketPath string

	cluster tnet.ClusterEvent
}

type BGPConfig struct {
	Binary string
	Path   string
}

func NewBGP(net *tnet.Net, config BGPConfig, logger *slog.Logger) (*BGP, error) {
	path := filepath.Clean(config.Path)
	if err := os.MkdirAll(path, os.ModePerm); err != nil {
		return nil, fmt.Errorf("error while ensuring bgp state directory exists: %w", err)
	}
	configPath := filepath.Join(path, "bird.conf")
	socketPath := filepath.Join(path, "bird.ctl")

	bgp := &BGP{
		logger: logger,
		net:    net,

		cmd:        cmd.NewCommand(logger.With("process", "bird"), slog.LevelDebug, slog.LevelWarn),
		config:     "",
		binary:     config.Binary,
		configPath: configPath,
		socketPath: socketPath,
	}

	if err := bgp.writeConfig(bgp.generateConfig()); err != nil {
		return nil, fmt.Errorf("error while writing empty config: %w", err)
	}

	if err := bgp.startBird(); err != nil {
		return nil, fmt.Errorf("error while starting bird: %w", err)
	}

	return bgp, nil
}

func (b *BGP) startBird() error {
	return b.cmd.Start(b.binary, "-f", "-c", b.configPath, "-s", b.socketPath)
}

func (b *BGP) reloadBird() error {
	return b.cmd.Signal(syscall.SIGHUP)
}

func (b *BGP) writeConfig(config string) error {
	if b.config == config {
		return nil
	}

	if err := os.WriteFile(b.configPath, []byte(config), ConfigPerm); err != nil {
		return fmt.Errorf("failed to write bird config: %w", err)
	}

	return nil
}

func (b *BGP) generateConfig() string {
	config := `
# Generated bird config
protocol device {
  scan time 10;
}
`

	for name, node := range b.cluster {
		_ = name
		_ = node
		// TODO: populate config based on node data
	}

	return config
}

func (b *BGP) configureBird() error {
	config := b.generateConfig()
	if err := b.writeConfig(config); err != nil {
		return fmt.Errorf("failed to configure bird: %w", err)
	}

	return b.reloadBird()
}

func (b *BGP) Run(ctx context.Context, notify run.Notify) error {
	sub := b.net.Cluster().Broker().Subscribe()
	defer sub.Unsubscribe()
	defer b.cmd.Stop()

	error := make(chan error)
	go func() {
		select {
		case <-ctx.Done():
		case error <- b.cmd.Wait():
		}
	}()

	notify.Notify()
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-error:
			return fmt.Errorf("bird process exited: %w", err)
		case event := <-sub.Chan():
			b.cluster = event
			if err := b.configureBird(); err != nil {
				return fmt.Errorf("error while configuring bird BGP daemon: %w", err)
			}
		}
	}
}
