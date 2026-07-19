package ldapsrv

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	"github.com/teapotovh/teapot/lib/observability"
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
type LDAPSrv[T any] struct {
	logger *slog.Logger

	address       string
	shutdownDelay time.Duration
	readTimeout   time.Duration
	writeTimeout  time.Duration

	initialState T
	listener     net.Listener
	handler      Handler[T]
	wg           sync.WaitGroup
	metrics      metrics
	tracer       trace.Tracer
	running      atomic.Bool
}

// NewServer return a LDAP Server.
func NewServer[T any](config LDAPSrvConfig, logger *slog.Logger) (*LDAPSrv[T], error) {
	srv := LDAPSrv[T]{
		logger: slog.New(NewContextHandler(logger.Handler())),

		tracer: observability.NoopTracer,

		address:       config.Address,
		shutdownDelay: config.ShutdownDelay,
		readTimeout:   config.ReadTimeout,
		writeTimeout:  config.WriteTimeout,
	}

	srv.initMetrics()

	return &srv, nil
}

// Register registers the handler for the server.
func (s *LDAPSrv[T]) Register(h Handler[T]) {
	if s.handler != nil {
		s.logger.Warn("overwriting ldap handler", "old", s.handler, "new", h)
	}

	s.handler = h
}

// Run implements run.Runnable.
func (s *LDAPSrv[T]) Run(ctx context.Context, notify run.Notify) (err error) {
	if s.handler == nil {
		return ErrMissingHandler
	}

	cfg := net.ListenConfig{}

	s.listener, err = cfg.Listen(ctx, "tcp", s.address)
	if err != nil {
		return fmt.Errorf("error while listening on tcp socket %q: %w", s.address, err)
	}

	s.running.Store(true)

	defer func() {
		s.running.Store(false)

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

			ch := make(chan unit)
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
			conn, err := s.listener.Accept()
			if nil != err {
				var ne *net.OpError
				if ok := errors.As(err, &ne); ok && ne.Timeout() {
					continue
				}

				s.logger.WarnContext(ctx, "error while handling incoming connection", "err", err)

				continue
			}

			ctx := context.WithValue(ctx, ContextKeyAddr, conn.RemoteAddr().String())

			uid, err := uuid.NewRandom()
			if err != nil {
				s.logger.WarnContext(ctx, "could not generate request id", "err", err)

				uid = uuid.UUID{}
			}

			ctx = context.WithValue(ctx, ContextKeyRequestID, uid)

			if err := s.setupConnection(ctx, conn, i); err != nil {
				s.logger.ErrorContext(ctx, "error while setting up connection", "err", err)
				continue
			}
		}
	}
}

// WithTracing implements observability.Tracing.
func (s *LDAPSrv[T]) WithTracing(_ trace.TracerProvider, tracer trace.Tracer) {
	s.tracer = tracer
}

func (s *LDAPSrv[T]) setupConnection(ctx context.Context, conn net.Conn, id int) error {
	if s.readTimeout > 0 {
		if err := conn.SetReadDeadline(time.Now().Add(s.readTimeout)); err != nil {
			return fmt.Errorf("error while setting read deadline: %w", err)
		}
	}

	if s.writeTimeout > 0 {
		if err := conn.SetWriteDeadline(time.Now().Add(s.writeTimeout)); err != nil {
			return fmt.Errorf("error while setting write deadline: %w", err)
		}
	}

	s.logger.DebugContext(ctx, "accepted connection")
	client := &client[T]{
		logger: s.logger,
		srv:    s,
		id:     id,
		rwc:    conn,
		br:     bufio.NewReader(conn),
		bw:     bufio.NewWriter(conn),
	}

	go client.serve(ctx)

	return nil
}
