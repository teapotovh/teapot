package ldapserver

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"
)

var ErrMissingHandler = errors.New("ldap server has no defined handler function")

// Server is an LDAP server.
type Server struct {
	logger *slog.Logger

	Listener     net.Listener
	ReadTimeout  time.Duration  // optional read timeout
	WriteTimeout time.Duration  // optional write timeout
	wg           sync.WaitGroup // group of goroutines (1 by client)

	// OnNewConnection, if non-nil, is called on new connections.
	// If it returns non-nil, the connection is closed.
	OnNewConnection func(c net.Conn) error

	// Handler handles ldap message received from client
	// it SHOULD "implement" RequestHandler interface
	Handler Handler
}

// NewServer return a LDAP Server
func NewServer(logger *slog.Logger) *Server {
	return &Server{
		logger: logger,
	}
}

// Handle registers the handler for the server.
func (s *Server) Handle(h Handler) {
	if s.Handler != nil {
		s.logger.Warn("overwriting ldap handler", "old", s.Handler, "new", h)
	}
	s.Handler = h
}

type Option func(*Server) error

// ListenAndServe listens on the TCP network address s.Addr and then
// calls `serve` to handle requests on incoming connections.
// When the context is canceled, the server gracefully closes all connections.
func (s *Server) ListenAndServe(ctx context.Context, addr string, options ...Option) (err error) {
	s.Listener, err = net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("error while listening on tcp socket: %w", err)
	}

	for _, option := range options {
		if err := option(s); err != nil {
			return fmt.Errorf("error while applying option: %w", err)
		}
	}

	return s.serve(ctx)
}

// WIthTLS returns an Option that wraps the TLS connection with TLS
func WithTLS(config *tls.Config) Option {
	return func(srv *Server) error {
		srv.Listener = tls.NewListener(srv.Listener, config)
		return nil
	}
}

// Handle requests messages on the ln listener
func (s *Server) serve(ctx context.Context) (err error) {
	defer func() {
		if err := s.Listener.Close(); err != nil {
			err = fmt.Errorf("error while closing ldap listener: %w", err)
		}
	}()

	if s.Handler == nil {
		return ErrMissingHandler
	}

	i := 0

	for {
		select {
		case <-ctx.Done():
			s.logger.Debug("context canceled, stopping server", "listener", s.Listener)
			return nil
		default:
		}

		i += 1
		ctx := context.WithValue(ctx, ContextKeyConnectionID, i)

		rw, err := s.Listener.Accept()
		if nil != err {
			if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
				continue
			}

			s.logger.WarnContext(ctx, "error while handling incoming connection", "err", err)
			continue
		}

		if s.ReadTimeout != 0 {
			if err := rw.SetReadDeadline(time.Now().Add(s.ReadTimeout)); err != nil {
				s.logger.WarnContext(ctx, "error while setting read deadline", "err", err)
				continue
			}
		}
		if s.WriteTimeout != 0 {
			if err := rw.SetWriteDeadline(time.Now().Add(s.WriteTimeout)); err != nil {
				s.logger.WarnContext(ctx, "error while setting write deadline", "err", err)
				continue
			}
		}

		s.logger.DebugContext(ctx, "accepted connection", "addr", rw.RemoteAddr().String())
		cli, err := s.newClient(rw, i)
		go cli.serve(ctx)
	}
}

// Return a new session with the connection
// client has a writer and reader buffer
func (s *Server) newClient(rwc net.Conn, id int) (c *client, err error) {
	c = &client{
		logger: s.logger.With("client", id),
		srv:    s,
		id:     id,
		rwc:    rwc,
		br:     bufio.NewReader(rwc),
		bw:     bufio.NewWriter(rwc),
	}
	return c, nil
}

// Wait waits for the termination of all LDAP client connections.
//
// Termination of the LDAP session is initiated by the server sending a
// Notice of Disconnection.  In this case, each
// protocol peer gracefully terminates the LDAP session by ceasing
// exchanges at the LDAP message layer, tearing down any SASL layer,
// tearing down any TLS layer, and closing the transport connection.
// A protocol peer may determine that the continuation of any
// communication would be pernicious, and in this case, it may abruptly
// terminate the session by ceasing communication and closing the
// transport connection.
// In either case, the LDAP session is terminated.
func (s *Server) Wait() {
	s.logger.Debug("gracefully closing client connections")
	s.wg.Wait()
}
