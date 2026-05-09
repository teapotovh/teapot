package ldapsrv

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/teapotovh/teapot/lib/run"
)

var ErrMissingHandler = errors.New("ldap server has no defined handler function")

type unit struct{}

type LDAPSrvConfig struct {
	Address       string
	ShutdownDelay time.Duration
	ReadTimeout   time.Duration
	WriteTimeout  time.Duration
}

// LDAPSrv is an LDAP server.
type LDAPSrv struct {
	logger *slog.Logger

	address       string
	shutdownDelay time.Duration
	readTimeout   time.Duration
	writeTimeout  time.Duration

	listener net.Listener
	handler  Handler
	wg       sync.WaitGroup
}

// NewServer return a LDAP Server.
func NewServer(config LDAPSrvConfig, logger *slog.Logger) *LDAPSrv {
	return &LDAPSrv{
		logger: logger,

		address:       config.Address,
		shutdownDelay: config.ShutdownDelay,
		readTimeout:   config.ReadTimeout,
		writeTimeout:  config.WriteTimeout,
	}
}

// Handle registers the handler for the server.
func (s *LDAPSrv) Handle(h Handler) {
	if s.handler != nil {
		s.logger.Warn("overwriting ldap handler", "old", s.handler, "new", h)
	}

	s.handler = h
}

// Wait waits for the termination of all LDAP client connections.
//
// Termination of the LDAP session is initiated by the server sending a
// Notice of Disconnection.  In this case, each
// protocol peer gracefully terminates the LDAP session by ceasing
// exchanges at the LDAP message layer, tearing down any SASL layer,
// and closing the transport connection.
// A protocol peer may determine that the continuation of any
// communication would be pernicious, and in this case, it may abruptly
// terminate the session by ceasing communication and closing the
// transport connection.
// In either case, the LDAP session is terminated.
func (s *LDAPSrv) Wait() {
}

// Run implements run.Runnable.
func (s *LDAPSrv) Run(ctx context.Context, notify run.Notify) (err error) {
	if s.handler == nil {
		return ErrMissingHandler
	}

	cfg := net.ListenConfig{}
	s.listener, err = cfg.Listen(ctx, "tcp", s.address)
	if err != nil {
		return fmt.Errorf("error while listening on tcp socket %q: %w", s.address, err)
	}

	defer func() {
		if lisErr := s.listener.Close(); lisErr != nil && err == nil {
			err = fmt.Errorf("error while closing ldap listener: %w", err)
		}
	}()

	notify.Notify()

	i := 0
	for {
		select {
		case <-ctx.Done():
			s.logger.DebugContext(ctx, "gracefully closing client connections")

			var ch chan unit
			defer close(ch)
			go func() {
				s.wg.Wait()
				ch <- unit{}
			}()

			ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(s.shutdownDelay))
			defer cancel()

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ch:
				return nil
			}

		default:
			s.listener.Accept()

			i += 1
			ctx := context.WithValue(ctx, ContextKeyConnectionID, i)

			rw, err := s.listener.Accept()
			if nil != err {
				var ne *net.OpError
				if ok := errors.As(err, &ne); ok && ne.Timeout() {
					continue
				}

				s.logger.WarnContext(ctx, "error while handling incoming connection", "err", err)

				continue
			}

			if s.readTimeout > 0 {
				if err := rw.SetReadDeadline(time.Now().Add(s.readTimeout)); err != nil {
					s.logger.WarnContext(ctx, "error while setting read deadline", "err", err)
					continue
				}
			}

			if s.writeTimeout > 0 {
				if err := rw.SetWriteDeadline(time.Now().Add(s.writeTimeout)); err != nil {
					s.logger.WarnContext(ctx, "error while setting write deadline", "err", err)
					continue
				}
			}

			s.logger.DebugContext(ctx, "accepted connection", "addr", rw.RemoteAddr().String())

			cli, err := s.newClient(rw, i)
			if err != nil {
				s.logger.WarnContext(ctx, "error while creating a new client for the connection", "err", err)
				continue
			}

			go cli.serve(ctx)
		}
	}
}

// Return a new session with the connection
// client has a writer and reader buffer.
func (s *LDAPSrv) newClient(rwc net.Conn, id int) (c *client, err error) {
	c = &client{
		logger: s.logger,
		srv:    s,
		id:     id,
		rwc:    rwc,
		br:     bufio.NewReader(rwc),
		bw:     bufio.NewWriter(rwc),
	}

	return c, nil
}
