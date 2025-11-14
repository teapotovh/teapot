package bgp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"path/filepath"
	"syscall"

	"github.com/teapotovh/teapot/lib/cmd"
	"github.com/teapotovh/teapot/lib/run"
	tnet "github.com/teapotovh/teapot/service/net"
)

const (
	ConfigPerm    = os.FileMode(0644)
	InternalBGPAS = 65001
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

	device    string
	cluster   tnet.ClusterEvent
	localNode *tnet.ClusterNode
}

type BGPConfig struct {
	Binary string
	Path   string
	Device string
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

		device: config.Device,
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
	if err := os.WriteFile(b.configPath, []byte(config), ConfigPerm); err != nil {
		return fmt.Errorf("failed to write bird config: %w", err)
	}

	b.config = config
	return nil
}

func (b *BGP) generateStaticProtocol() string {
	result := ""
	if b.localNode != nil {
		// TODO: loop ove CIDR
		result += fmt.Sprintf(`
protocol static {
  ipv6;
  route %s/128 via "%s";
}
`, b.localNode.CIDRs[0], b.device)
	}

	if len(b.cluster) > 1 {
		result += `
protocol static peers {
	ipv6;
`
		for name, node := range b.cluster {
			if !node.IsLocal {
				result += fmt.Sprintf("  # node: %s, static route\n", name)
				result += fmt.Sprintf("  route %s/128 via \"%s\";\n\n", node.InternalIP, b.device)
			}
		}
		result += "}\n\n"
	}

	return result
}

func (b *BGP) generateBGPProtocol() string {
	result := ""
	i := 0
	for name, node := range b.cluster {
		if !node.IsLocal {
			result += fmt.Sprintf("# node: %s, bgp route\n", name)
			result += fmt.Sprintf("protocol bgp node%d {\n", i)
			result += fmt.Sprintf("  neighbor %s as %d;\n", node.InternalIP, InternalBGPAS)
			result += fmt.Sprintf("  local as %d;\n", InternalBGPAS)
			result += "  ipv6 {\n"
			result += "    import all;\n    export all;\n"
			result += "  };\n"
			result += "}\n\n"

			i += 1
		}
	}

	return result
}

func (b *BGP) generateConfig() string {
	var id [4]byte
	if b.localNode != nil {
		bytes := b.localNode.InternalIP.As16()
		id = [4]byte(bytes[12:])
	} else {
		id = [4]byte{1, 1, 1, 1}
	}
	router := netip.AddrFrom4(id)

	config := fmt.Sprintf(`
router id %s;

protocol device {}

protocol kernel {
    persist;
    ipv6 {
        export all;
        import all;
    };
}

%s

%s
`, router, b.generateStaticProtocol(), b.generateBGPProtocol())

	return config
}

func (b *BGP) configureBird() error {
	config := b.generateConfig()
	if b.config == config {
		b.logger.Debug("update caused no change in bird config")
		return nil
	}

	if err := b.writeConfig(config); err != nil {
		return fmt.Errorf("failed to configure bird: %w", err)
	}

	if err := b.reloadBird(); err != nil {
		return fmt.Errorf("error while reloading bird configuration: %w", err)
	}

	b.logger.Info("updated bird with new information")
	return nil
}

func (b *BGP) Run(ctx context.Context, notify run.Notify) error {
	csub := b.net.Cluster().Broker().Subscribe()
	defer csub.Unsubscribe()

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
		case cluster := <-csub.Chan():
			b.cluster = cluster
			for _, node := range b.cluster {
				if node.IsLocal {
					b.localNode = &node
				}
			}

			if err := b.configureBird(); err != nil {
				return fmt.Errorf("error while configuring bird BGP daemon: %w", err)
			}
		}
	}
}
