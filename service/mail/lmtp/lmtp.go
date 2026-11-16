package lmtp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/emersion/go-smtp"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/teapotovh/teapot/lib/observability"
	"github.com/teapotovh/teapot/lib/run"
	mailservice "github.com/teapotovh/teapot/service/mail"
	"github.com/teapotovh/teapot/service/mail/store"
)

const (
	networkTCP      = "tcp"
	networkUnix     = "unix"
	defaultNetwork  = networkTCP
	defaultDomain   = "localhost"
	statusSuccess   = "success"
	statusError     = "error"
	unknownRcptText = "Unknown recipient"
	temporaryText   = "Temporary error while storing message"
	tooLargeText    = "Message too large"
)

var (
	// ErrUnknownRecipient is returned when a recipient does not exist.
	ErrUnknownRecipient = mailservice.ErrRecipientNotFound

	// ErrTemporary is returned when a recipient lookup failed temporarily.
	ErrTemporary = mailservice.ErrRecipientTemporary

	errUnknownRecipient = &smtp.SMTPError{
		Code:         550,
		EnhancedCode: smtp.EnhancedCode{5, 1, 1},
		Message:      unknownRcptText,
	}
	errTemporaryStore = &smtp.SMTPError{
		Code:         451,
		EnhancedCode: smtp.EnhancedCode{4, 2, 0},
		Message:      temporaryText,
	}
	errMessageTooLarge = &smtp.SMTPError{
		Code:         552,
		EnhancedCode: smtp.EnhancedCode{5, 3, 4},
		Message:      tooLargeText,
	}

	errMissingStore   = errors.New("mail store is required")
	errUnsupportedNet = errors.New("unsupported network")
)

// RecipientResolver resolves a recipient address to a mailbox owner.
type RecipientResolver interface {
	ResolveRecipient(ctx context.Context, address string) (owner string, err error)
}

// StaticResolver is a simple RecipientResolver backed by a map of address to
// owner. It is intended for tests.
type StaticResolver map[string]string

// ResolveRecipient implements RecipientResolver.
func (r StaticResolver) ResolveRecipient(_ context.Context, address string) (string, error) {
	address = strings.TrimSpace(address)

	owner, ok := r[address]
	if !ok || owner == "" {
		return "", ErrUnknownRecipient
	}

	return owner, nil
}

// Config configures an LMTP server.
type Config struct {
	Addr            string
	Network         string
	Domain          string
	MaxMessageBytes int64
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownDelay   time.Duration
	Listener        net.Listener
}

// Backend implements smtp.Backend for LMTP delivery.
type Backend struct {
	mail            *mailservice.Mail
	logger          *slog.Logger
	maxMessageBytes int64
	metrics         *metrics
}

// NewSession implements smtp.Backend.
func (b *Backend) NewSession(*smtp.Conn) (smtp.Session, error) {
	return &session{backend: b}, nil
}

type recipient struct {
	address string
	owner   string
}

type session struct {
	backend    *Backend
	from       string
	recipients []recipient
}

// Reset implements smtp.Session.
func (s *session) Reset() {
	s.from = ""
	s.recipients = nil
}

// Logout implements smtp.Session.
func (s *session) Logout() error {
	s.Reset()

	return nil
}

// Mail implements smtp.Session.
func (s *session) Mail(from string, _ *smtp.MailOptions) error {
	s.from = from

	return nil
}

// Rcpt implements smtp.Session.
func (s *session) Rcpt(to string, _ *smtp.RcptOptions) error {
	to = strings.TrimSpace(to)

	owner, err := s.backend.mail.ResolveRecipient(context.Background(), to)
	if err != nil {
		switch {
		case errors.Is(err, ErrUnknownRecipient):
			return errUnknownRecipient
		case errors.Is(err, ErrTemporary):
			return errTemporaryStore
		default:
			s.backend.logger.ErrorContext(context.Background(),
				"error while resolving recipient", "address", to, "err", err)

			return errTemporaryStore
		}
	}

	s.recipients = append(s.recipients, recipient{address: to, owner: owner})

	return nil
}

// Data implements smtp.Session.
func (s *session) Data(r io.Reader) error {
	data, err := s.readMessage(r)
	if err != nil {
		return err
	}

	for _, rcpt := range s.recipients {
		if err := s.backend.deliver(context.Background(), s.from, rcpt.owner, data); err != nil {
			return deliveryError(err)
		}
	}

	return nil
}

// LMTPData implements smtp.LMTPSession.
func (s *session) LMTPData(r io.Reader, status smtp.StatusCollector) error {
	data, err := s.readMessage(r)
	if err != nil {
		return err
	}

	for _, rcpt := range s.recipients {
		if err := s.backend.deliver(context.Background(), s.from, rcpt.owner, data); err != nil {
			status.SetStatus(rcpt.address, deliveryError(err))

			continue
		}

		status.SetStatus(rcpt.address, nil)
	}

	return nil
}

func (s *session) readMessage(r io.Reader) ([]byte, error) {
	maxMessageBytes := s.backend.maxMessageBytes

	reader := r
	if maxMessageBytes > 0 {
		reader = io.LimitReader(r, maxMessageBytes+1)
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		if errors.Is(err, smtp.ErrDataTooLarge) {
			return nil, errMessageTooLarge
		}

		return nil, fmt.Errorf("error while reading message data: %w", err)
	}

	if maxMessageBytes > 0 && int64(len(data)) > maxMessageBytes {
		return nil, errMessageTooLarge
	}

	return data, nil
}

func (b *Backend) deliver(ctx context.Context, from, owner string, data []byte) error {
	mailbox, err := b.inbox(ctx, owner)
	if err != nil {
		b.metrics.observe(err)
		b.logger.ErrorContext(ctx, "error while getting inbox", "from", from, "owner", owner, "err", err)

		return err
	}

	if _, err := b.mail.Store().Append(ctx, mailbox.ID, data, nil, time.Now()); err != nil {
		b.metrics.observe(err)
		b.logger.ErrorContext(ctx, "error while appending message", "from", from, "owner", owner, "err", err)

		return err
	}

	b.metrics.observe(nil)

	return nil
}

func (b *Backend) inbox(ctx context.Context, owner string) (*store.Mailbox, error) {
	mailbox, err := b.mail.Store().GetMailbox(ctx, owner, store.InboxName)
	if err == nil {
		return mailbox, nil
	}

	if !errors.Is(err, store.ErrMailboxNotFound) {
		return nil, err
	}

	mailbox, err = b.mail.Store().CreateMailbox(ctx, owner, store.InboxName, "\\Inbox")
	if err == nil {
		return mailbox, nil
	}

	if errors.Is(err, store.ErrMailboxAlreadyExist) {
		return b.mail.Store().GetMailbox(ctx, owner, store.InboxName)
	}

	return nil, err
}

func deliveryError(err error) error {
	if smtpErr, ok := errors.AsType[*smtp.SMTPError](err); ok {
		return smtpErr
	}

	return errTemporaryStore
}

type smtpLogger struct {
	logger *slog.Logger
}

func (l smtpLogger) Printf(format string, v ...any) {
	l.logger.Debug("smtp server log", "message", fmt.Sprintf(format, v...))
}

func (l smtpLogger) Println(v ...any) {
	l.logger.Debug("smtp server log", "message", fmt.Sprint(v...))
}

// Server is an LMTP server.
type Server struct {
	config Config
	logger *slog.Logger

	inner   *smtp.Server
	backend *Backend
	metrics metrics

	mu       sync.RWMutex
	listener net.Listener

	running atomic.Bool
	started atomic.Bool
}

// NewServer creates a new LMTP server.
func NewServer(config Config, mail *mailservice.Mail, logger *slog.Logger) (*Server, error) {
	if mail == nil {
		return nil, errMissingStore
	}

	if config.Network == "" {
		config.Network = defaultNetwork
	}

	if config.Network != networkTCP && config.Network != networkUnix {
		return nil, fmt.Errorf("%w: %q", errUnsupportedNet, config.Network)
	}

	if config.Domain == "" {
		config.Domain = defaultDomain
	}

	if logger == nil {
		logger = slog.Default()
	}

	srv := &Server{
		config: config,
		logger: logger,
	}
	srv.metrics.initMetrics()
	srv.backend = &Backend{
		mail:            mail,
		logger:          logger,
		maxMessageBytes: config.MaxMessageBytes,
		metrics:         &srv.metrics,
	}

	inner := smtp.NewServer(srv.backend)
	inner.Network = config.Network
	inner.Addr = config.Addr
	inner.Domain = config.Domain
	inner.LMTP = true
	inner.MaxMessageBytes = config.MaxMessageBytes
	inner.ReadTimeout = config.ReadTimeout
	inner.WriteTimeout = config.WriteTimeout
	inner.ErrorLog = smtpLogger{logger: logger}

	srv.inner = inner

	return srv, nil
}

// Listener returns the listener used by the server, or nil if it is not
// running.
func (s *Server) Listener() net.Listener {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.listener
}

// Addr returns the address the server is listening on, or nil if it is not
// running.
func (s *Server) Addr() net.Addr {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.listener == nil {
		return nil
	}

	return s.listener.Addr()
}

// Run implements run.Runnable.
//
//nolint:gocyclo
func (s *Server) Run(ctx context.Context, notify run.Notify) (err error) {
	listener := s.config.Listener
	if listener == nil {
		var listenConfig net.ListenConfig

		var err error

		listener, err = listenConfig.Listen(ctx, s.config.Network, s.config.Addr)
		if err != nil {
			return fmt.Errorf("error while listening on %s socket %q: %w", s.config.Network, s.config.Addr, err)
		}
	}

	s.mu.Lock()
	s.listener = listener
	s.mu.Unlock()

	s.started.Store(true)
	s.running.Store(true)

	defer func() {
		s.running.Store(false)

		s.mu.Lock()
		listener := s.listener
		s.listener = nil
		s.mu.Unlock()

		if listener != nil {
			if closeErr := listener.Close(); closeErr != nil && !errors.Is(closeErr, net.ErrClosed) && err == nil {
				err = fmt.Errorf("error while closing listener: %w", closeErr)
			}
		}
	}()

	notify.Notify()

	serveErr := make(chan error, 1)

	go func() {
		serveErr <- s.inner.Serve(listener)
	}()

	select {
	case <-ctx.Done():
		s.logger.DebugContext(ctx, "gracefully closing LMTP server")

		if closeErr := s.Close(); closeErr != nil {
			s.logger.WarnContext(ctx, "error while closing LMTP server", "err", closeErr)
		}

		if err := <-serveErr; err != nil && !errors.Is(err, net.ErrClosed) && !errors.Is(err, smtp.ErrServerClosed) {
			return fmt.Errorf("error while serving LMTP: %w", err)
		}

		return nil
	case err := <-serveErr:
		if err == nil || errors.Is(err, net.ErrClosed) || errors.Is(err, smtp.ErrServerClosed) {
			return nil
		}

		return fmt.Errorf("error while serving LMTP: %w", err)
	}
}

// Close closes the server and all active connections.
func (s *Server) Close() error {
	s.mu.RLock()
	listener := s.listener
	s.mu.RUnlock()

	s.running.Store(false)

	err := s.inner.Close()
	if errors.Is(err, smtp.ErrServerClosed) {
		err = nil
	}

	if listener != nil {
		if closeErr := listener.Close(); closeErr != nil && !errors.Is(closeErr, net.ErrClosed) && err == nil {
			err = closeErr
		}
	}

	return err
}

// Metrics implements observability.Metrics.
func (s *Server) Metrics() []prometheus.Collector {
	return []prometheus.Collector{s.metrics.messages}
}

// ReadinessChecks implements observability.ReadinessChecks.
func (s *Server) ReadinessChecks() map[string]observability.Check {
	return map[string]observability.Check{
		"lmtp/started": observability.CheckFunc(func(context.Context) error {
			if !s.started.Load() {
				return ErrNotStarted
			}

			return nil
		}),
	}
}

// LivenessChecks implements observability.LivenessChecks.
func (s *Server) LivenessChecks() map[string]observability.Check {
	return map[string]observability.Check{
		"lmtp/running": observability.CheckFunc(func(context.Context) error {
			if !s.running.Load() {
				return ErrNotRunning
			}

			return nil
		}),
	}
}

var (
	// ErrNotStarted is returned when the LMTP server has not started.
	ErrNotStarted = errors.New("LMTP server has not started")

	// ErrNotRunning is returned when the LMTP server is not running.
	ErrNotRunning = errors.New("LMTP server is not running")
)
