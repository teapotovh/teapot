package imap

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync/atomic"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/teapotovh/teapot/lib/observability"
	"github.com/teapotovh/teapot/lib/run"
	mailservice "github.com/teapotovh/teapot/service/mail"
)

const defaultPollInterval = 10 * time.Second

type ServerConfig struct {
	Addr          string
	Network       string
	InsecureAuth  bool
	PollInterval  time.Duration
	ShutdownDelay time.Duration
	Listener      net.Listener
	TLS           *tls.Config
}

type Server struct {
	logger   *slog.Logger
	mail     *mailservice.Mail
	inner    *imapserver.Server
	listener net.Listener

	running       atomic.Bool
	started       atomic.Bool
	shutdownDelay time.Duration
}

func NewServer(config ServerConfig, mail *mailservice.Mail, logger *slog.Logger) (*Server, error) {
	network := config.Network
	if network == "" {
		network = "tcp"
	}

	listener := config.Listener
	if listener == nil {
		addr := config.Addr
		if addr == "" {
			addr = ":143"
		}

		var err error

		listener, err = net.Listen(network, addr)
		if err != nil {
			return nil, fmt.Errorf("error while listening for IMAP connections: %w", err)
		}
	}

	if config.TLS != nil {
		listener = tls.NewListener(listener, config.TLS)
	}

	caps := imap.CapSet{
		imap.CapIMAP4rev1:    {},
		imap.CapIMAP4rev2:    {},
		imap.CapUIDPlus:      {},
		imap.CapMove:         {},
		imap.CapNamespace:    {},
		imap.CapUnselect:     {},
		imap.CapIdle:         {},
		imap.CapEnable:       {},
		imap.CapListExtended: {},
		imap.CapListStatus:   {},
		imap.CapSpecialUse:   {},
		imap.CapLiteralPlus:  {},
		imap.CapSASLIR:       {},
	}

	inner := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return NewSession(mail, logger, config.PollInterval), &imapserver.GreetingData{}, nil
		},
		Caps:         caps,
		TLSConfig:    config.TLS,
		InsecureAuth: config.InsecureAuth,
	})

	return &Server{
		logger:        logger,
		mail:          mail,
		inner:         inner,
		listener:      listener,
		shutdownDelay: config.ShutdownDelay,
	}, nil
}

func (s *Server) Addr() net.Addr {
	return s.listener.Addr()
}

func (s *Server) Listener() net.Listener {
	return s.listener
}

func (s *Server) Run(ctx context.Context, notify run.Notify) error {
	s.started.Store(true)
	s.running.Store(true)

	errCh := make(chan error, 1)

	go func() {
		errCh <- s.inner.Serve(s.listener)

		s.running.Store(false)
	}()

	notify.Notify()

	select {
	case <-ctx.Done():
		s.running.Store(false)

		if err := s.inner.Close(); err != nil {
			return fmt.Errorf("error while closing IMAP server: %w", err)
		}

		return nil
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("error while serving IMAP: %w", err)
		}

		return nil
	}
}

func (s *Server) Metrics() []prometheus.Collector {
	return nil
}

func (s *Server) ReadinessChecks() map[string]observability.Check {
	return map[string]observability.Check{
		"imap/started": observability.CheckFunc(func(context.Context) error {
			if !s.started.Load() {
				return ErrNotStarted
			}

			return nil
		}),
	}
}

func (s *Server) LivenessChecks() map[string]observability.Check {
	return map[string]observability.Check{
		"imap/running": observability.CheckFunc(func(context.Context) error {
			if !s.running.Load() {
				return ErrNotRunning
			}

			return nil
		}),
	}
}

var (
	// ErrNotStarted is returned when the IMAP server has not started.
	ErrNotStarted = errors.New("IMAP server has not started")

	// ErrNotRunning is returned when the IMAP server is not running.
	ErrNotRunning = errors.New("IMAP server is not running")
)

var (
	_ run.Runnable                  = (*Server)(nil)
	_ observability.Metrics         = (*Server)(nil)
	_ observability.ReadinessChecks = (*Server)(nil)
	_ observability.LivenessChecks  = (*Server)(nil)
)
