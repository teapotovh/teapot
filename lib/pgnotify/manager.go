package pgnotify

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	// ErrInvalidConfig is returned when the connection config is nil.
	ErrInvalidConfig = errors.New("invalid connection config")
	// ErrNotListening is returned when Next is called before Listen.
	ErrNotListening = errors.New("not listening")
)

const (
	// KeepaliveCheckTime is the interval between connection keepalive checks.
	KeepaliveCheckTime = 10 * time.Minute
	// PingTimeout is the timeout for a connection keepalive check.
	PingTimeout = 5 * time.Second
)

// Conn is a dedicated connection used to receive notifications.
type Conn interface {
	// WaitForNotification waits for the next notification.
	WaitForNotification(ctx context.Context) (*pgconn.Notification, error)
	// Exec runs a query on the connection.
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	// Ping checks that the connection is alive.
	Ping(ctx context.Context) error
	// Close closes the connection.
	Close(ctx context.Context) error
}

// Executor runs a query inside a transaction.
type Executor interface {
	// Exec runs a query.
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// Manager owns a dedicated connection for PostgreSQL notifications.
type Manager struct {
	logger  *slog.Logger
	id      uuid.UUID
	connect func(ctx context.Context) (Conn, error)

	mu       sync.Mutex
	conn     Conn
	channels []string
	listened map[string]struct{}
	buffers  map[string]*buffer
}

// buffer collects the frames of a chunked payload for one channel.
type buffer struct {
	data []byte
	next uint64
}

// NewManager returns a manager that uses a dedicated connection for notifications.
func NewManager(config *pgx.ConnConfig, logger *slog.Logger) (*Manager, error) {
	if config == nil {
		return nil, ErrInvalidConfig
	}

	config = config.Copy()

	return newManager(logger, uuid.New(), func(ctx context.Context) (Conn, error) {
		return pgx.ConnectConfig(ctx, config)
	}), nil
}

func newManager(
	logger *slog.Logger,
	id uuid.UUID,
	connect func(ctx context.Context) (Conn, error),
) *Manager {
	return &Manager{
		logger:   logger,
		id:       id,
		connect:  connect,
		listened: map[string]struct{}{},
		buffers:  map[string]*buffer{},
	}
}

// Listen registers a channel and starts listening on it. Call Listen before Next.
func (m *Manager) Listen(ctx context.Context, channel string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.listened[channel]; !ok {
		m.channels = append(m.channels, channel)
		m.listened[channel] = struct{}{}
	}

	if m.conn == nil {
		if err := m.connectLocked(ctx); err != nil {
			return fmt.Errorf("error while connecting: %w", err)
		}

		return nil
	}

	if err := listen(ctx, m.conn, channel); err != nil {
		return fmt.Errorf("error while listening on channel %q: %w", channel, err)
	}

	m.logger.InfoContext(ctx, "listening on channel", "channel", channel)

	return nil
}

// Notify sends a notification inside the given transaction.
func (m *Manager) Notify(ctx context.Context, tx Executor, channel string, payload []byte) error {
	frames := encode(payload, m.id)

	for _, frame := range frames {
		if _, err := tx.Exec(ctx, "SELECT pg_notify($1, $2);", channel, frame); err != nil {
			return fmt.Errorf("error while sending notification on channel %q: %w", channel, err)
		}
	}

	m.logger.InfoContext(ctx, "sent notification", "channel", channel, "bytes", len(payload), "frames", len(frames))

	return nil
}

// Next waits for the next complete notification.
func (m *Manager) Next(ctx context.Context) (string, []byte, error) {
	conn, err := m.connection()
	if err != nil {
		return "", nil, err
	}

	for {
		waitCtx, cancel := context.WithTimeout(ctx, KeepaliveCheckTime)
		msg, err := conn.WaitForNotification(waitCtx)

		cancel()

		if err != nil {
			if ctx.Err() != nil {
				return "", nil, ctx.Err()
			}

			if errors.Is(err, context.DeadlineExceeded) {
				if err := m.ping(ctx, conn); err == nil {
					continue
				}
			}

			return "", nil, fmt.Errorf("error while waiting for notification: %w", err)
		}

		frame, err := decodeFrame(msg.Payload)
		if err != nil {
			return "", nil, fmt.Errorf("error while decoding notification on channel %q: %w", msg.Channel, err)
		}

		if frame.sender == m.id {
			continue
		}

		channel, payload, complete, err := m.receive(msg.Channel, frame)
		if err != nil {
			return "", nil, err
		}

		if complete {
			return channel, payload, nil
		}
	}
}

// Close closes the listen connection.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.conn == nil {
		return nil
	}

	conn := m.conn
	m.conn = nil
	clear(m.buffers)

	if err := conn.Close(context.Background()); err != nil {
		return fmt.Errorf("error while closing listen connection: %w", err)
	}

	return nil
}

func (m *Manager) connection() (Conn, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.conn == nil {
		return nil, ErrNotListening
	}

	return m.conn, nil
}

func (m *Manager) connectLocked(ctx context.Context) error {
	conn, err := m.connect(ctx)
	if err != nil {
		return fmt.Errorf("error while opening listen connection: %w", err)
	}

	for _, channel := range m.channels {
		if err := listen(ctx, conn, channel); err != nil {
			_ = conn.Close(context.Background())

			return fmt.Errorf("error while listening on channel %q: %w", channel, err)
		}
	}

	m.conn = conn
	clear(m.buffers)
	m.logger.InfoContext(ctx, "connected for notifications", "channels", len(m.channels))

	return nil
}

func (m *Manager) receive(channel string, frame frame) (string, []byte, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	buffered := m.buffers[channel]
	if buffered == nil {
		if frame.index != 0 {
			return "", nil, false, fmt.Errorf(
				"%w: expected first frame on channel %q, got index %d",
				ErrInvalidFrame,
				channel,
				frame.index,
			)
		}

		if frame.more {
			m.buffers[channel] = &buffer{data: frame.data, next: 1}

			return "", nil, false, nil
		}

		return channel, frame.data, true, nil
	}

	if uint64(frame.index) != buffered.next {
		delete(m.buffers, channel)

		return "", nil, false, fmt.Errorf(
			"%w: unexpected frame index %d on channel %q, expected %d",
			ErrInvalidFrame,
			frame.index,
			channel,
			buffered.next,
		)
	}

	buffered.data = append(buffered.data, frame.data...)
	buffered.next++

	if frame.more {
		return "", nil, false, nil
	}

	delete(m.buffers, channel)

	return channel, buffered.data, true, nil
}

func (m *Manager) ping(ctx context.Context, conn Conn) error {
	pingCtx, cancel := context.WithTimeout(ctx, PingTimeout)
	defer cancel()

	return conn.Ping(pingCtx)
}

func listen(ctx context.Context, conn Conn, channel string) error {
	_, err := conn.Exec(ctx, "LISTEN "+pgx.Identifier{channel}.Sanitize())
	return err
}

var (
	// Ensure *pgx.Conn implements Conn.
	_ Conn = (*pgx.Conn)(nil)
	// Ensure pgx.Tx implements Executor.
	_ Executor = pgx.Tx(nil)
)
