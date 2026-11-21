package pgnotify

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

type fakeWait struct {
	msg *pgconn.Notification
	err error
}

var (
	// ErrTestConnection is used to simulate a connection failure.
	ErrTestConnection = errors.New("test connection failure")
	// ErrTestTransaction is used to simulate a transaction failure.
	ErrTestTransaction = errors.New("test transaction failure")
)

type fakeConn struct {
	waits  []fakeWait
	execs  []string
	pings  int
	closed bool
}

// Ensure *fakeConn implements Conn.
var _ Conn = (*fakeConn)(nil)

func (f *fakeConn) WaitForNotification(ctx context.Context) (*pgconn.Notification, error) {
	if len(f.waits) == 0 {
		<-ctx.Done()
		return nil, ctx.Err()
	}

	wait := f.waits[0]
	f.waits = f.waits[1:]

	return wait.msg, wait.err
}

func (f *fakeConn) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	f.execs = append(f.execs, sql)
	return pgconn.CommandTag{}, nil
}

func (f *fakeConn) Ping(_ context.Context) error {
	f.pings++
	return nil
}

func (f *fakeConn) Close(_ context.Context) error {
	f.closed = true
	return nil
}

type fakeExec struct {
	calls [][]any
	err   error
}

// Ensure *fakeExec implements Executor.
var _ Executor = (*fakeExec)(nil)

func (f *fakeExec) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	f.calls = append(f.calls, append([]any{sql}, args...))
	if f.err != nil {
		return pgconn.CommandTag{}, f.err
	}

	return pgconn.CommandTag{}, nil
}

func testContext(t *testing.T) context.Context {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	return ctx
}

func newTestManager(connect func(ctx context.Context) (Conn, error)) *Manager {
	return newManager(slog.New(slog.DiscardHandler), testSender, connect)
}

func TestNewManagerInvalidConfig(t *testing.T) {
	t.Parallel()

	if _, err := NewManager(nil, slog.New(slog.DiscardHandler)); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("got error %v, expected ErrInvalidConfig", err)
	}
}

func TestListen(t *testing.T) {
	t.Parallel()

	conn := &fakeConn{}
	manager := newTestManager(func(context.Context) (Conn, error) {
		return conn, nil
	})

	ctx := testContext(t)

	if err := manager.Listen(ctx, "teapot"); err != nil {
		t.Fatalf("error while listening: %v", err)
	}

	if err := manager.Listen(ctx, "other"); err != nil {
		t.Fatalf("error while listening: %v", err)
	}

	expected := []string{`LISTEN "teapot"`, `LISTEN "other"`}
	if len(conn.execs) != len(expected) {
		t.Fatalf("got %d LISTEN statements, expected %d", len(conn.execs), len(expected))
	}

	for i := range expected {
		if conn.execs[i] != expected[i] {
			t.Fatalf("got %q, expected %q", conn.execs[i], expected[i])
		}
	}
}

func TestNextAssemblesChunks(t *testing.T) {
	t.Parallel()

	payloadA := bytes.Repeat([]byte("a"), ChunkSize+1)
	payloadB := []byte("payload b")
	framesA := encode(payloadA, otherSender)
	framesB := encode(payloadB, otherSender)

	conn := &fakeConn{
		waits: []fakeWait{
			{msg: &pgconn.Notification{Channel: "a", Payload: framesA[0]}},
			{msg: &pgconn.Notification{Channel: "b", Payload: framesB[0]}},
			{msg: &pgconn.Notification{Channel: "a", Payload: framesA[1]}},
		},
	}

	manager := newTestManager(func(context.Context) (Conn, error) {
		return conn, nil
	})

	ctx := testContext(t)

	if err := manager.Listen(ctx, "a"); err != nil {
		t.Fatalf("error while listening: %v", err)
	}

	if err := manager.Listen(ctx, "b"); err != nil {
		t.Fatalf("error while listening: %v", err)
	}

	channel, payload, err := manager.Next(ctx)
	if err != nil {
		t.Fatalf("error while waiting for a notification: %v", err)
	}

	if channel != "b" || !bytes.Equal(payload, payloadB) {
		t.Fatalf(
			"got channel %q with %d bytes, expected channel \"b\" with %d bytes",
			channel,
			len(payload),
			len(payloadB),
		)
	}

	channel, payload, err = manager.Next(ctx)
	if err != nil {
		t.Fatalf("error while waiting for a notification: %v", err)
	}

	if channel != "a" || !bytes.Equal(payload, payloadA) {
		t.Fatalf(
			"got channel %q with %d bytes, expected channel \"a\" with %d bytes",
			channel,
			len(payload),
			len(payloadA),
		)
	}
}

func TestNextIgnoresOwnMessages(t *testing.T) {
	t.Parallel()

	ownFrames := encode(bytes.Repeat([]byte("a"), ChunkSize+1), testSender)
	otherFrames := encode([]byte("other"), otherSender)

	waits := make([]fakeWait, 0, len(ownFrames)+1)
	for _, raw := range ownFrames {
		waits = append(waits, fakeWait{msg: &pgconn.Notification{Channel: "a", Payload: raw}})
	}

	waits = append(waits, fakeWait{msg: &pgconn.Notification{Channel: "a", Payload: otherFrames[0]}})

	conn := &fakeConn{waits: waits}

	manager := newTestManager(func(context.Context) (Conn, error) {
		return conn, nil
	})

	ctx := testContext(t)

	if err := manager.Listen(ctx, "a"); err != nil {
		t.Fatalf("error while listening: %v", err)
	}

	_, payload, err := manager.Next(ctx)
	if err != nil {
		t.Fatalf("error while waiting for a notification: %v", err)
	}

	if !bytes.Equal(payload, []byte("other")) {
		t.Fatalf("got payload %q, expected \"other\"", payload)
	}
}

func TestNextPingsOnWaitTimeout(t *testing.T) {
	t.Parallel()

	payload := []byte("payload")
	frames := encode(payload, otherSender)

	conn := &fakeConn{
		waits: []fakeWait{
			{err: context.DeadlineExceeded},
			{msg: &pgconn.Notification{Channel: "a", Payload: frames[0]}},
		},
	}

	manager := newTestManager(func(context.Context) (Conn, error) {
		return conn, nil
	})

	ctx := testContext(t)

	if err := manager.Listen(ctx, "a"); err != nil {
		t.Fatalf("error while listening: %v", err)
	}

	if _, _, err := manager.Next(ctx); err != nil {
		t.Fatalf("error while waiting for a notification: %v", err)
	}

	if conn.pings != 1 {
		t.Fatalf("got %d pings, expected 1", conn.pings)
	}
}

func TestNextReturnsConnectionError(t *testing.T) {
	t.Parallel()

	conn := &fakeConn{
		waits: []fakeWait{
			{err: ErrTestConnection},
		},
	}

	connections := 0
	manager := newTestManager(func(context.Context) (Conn, error) {
		connections++
		return conn, nil
	})

	ctx := testContext(t)

	if err := manager.Listen(ctx, "a"); err != nil {
		t.Fatalf("error while listening: %v", err)
	}

	if _, _, err := manager.Next(ctx); !errors.Is(err, ErrTestConnection) {
		t.Fatalf("got error %v, expected %v", err, ErrTestConnection)
	}

	if connections != 1 {
		t.Fatalf("got %d connections, expected no reconnection", connections)
	}
}

func TestNextRejectsUnexpectedIndex(t *testing.T) {
	t.Parallel()

	conn := &fakeConn{
		waits: []fakeWait{
			{
				msg: &pgconn.Notification{
					Channel: "a",
					Payload: encodeFrame(frame{data: []byte("payload"), sender: otherSender, index: 1}),
				},
			},
		},
	}

	manager := newTestManager(func(context.Context) (Conn, error) {
		return conn, nil
	})

	ctx := testContext(t)

	if err := manager.Listen(ctx, "a"); err != nil {
		t.Fatalf("error while listening: %v", err)
	}

	if _, _, err := manager.Next(ctx); !errors.Is(err, ErrInvalidFrame) {
		t.Fatalf("got error %v, expected ErrInvalidFrame", err)
	}
}

func TestNotifySplitsFrames(t *testing.T) {
	t.Parallel()

	payload := bytes.Repeat([]byte{0}, 2*ChunkSize+1)

	manager := newTestManager(func(context.Context) (Conn, error) {
		return &fakeConn{}, nil
	})

	exec := &fakeExec{}

	if err := manager.Notify(testContext(t), exec, "a", payload); err != nil {
		t.Fatalf("error while notifying: %v", err)
	}

	if len(exec.calls) != 3 {
		t.Fatalf("got %d notifications, expected 3", len(exec.calls))
	}

	var decoded []byte

	for _, call := range exec.calls {
		if call[0] != "SELECT pg_notify($1, $2);" {
			t.Fatalf("got query %q", call[0])
		}

		if call[1] != "a" {
			t.Fatalf("got channel %v, expected \"a\"", call[1])
		}

		raw, ok := call[2].(string)
		if !ok {
			t.Fatalf("got payload type %T, expected string", call[2])
		}

		frame, err := decodeFrame(raw)
		if err != nil {
			t.Fatalf("error while decoding frame: %v", err)
		}

		if frame.sender != testSender {
			t.Fatalf("got sender %v, expected %v", frame.sender, testSender)
		}

		decoded = append(decoded, frame.data...)
	}

	if !bytes.Equal(decoded, payload) {
		t.Fatalf("decoded payload does not match the original")
	}
}

func TestNotifyError(t *testing.T) {
	t.Parallel()

	exec := &fakeExec{err: ErrTestTransaction}

	manager := newTestManager(func(context.Context) (Conn, error) {
		return &fakeConn{}, nil
	})

	err := manager.Notify(testContext(t), exec, "a", []byte("payload"))
	if !errors.Is(err, ErrTestTransaction) {
		t.Fatalf("got error %v, expected %v", err, ErrTestTransaction)
	}
}

func TestClose(t *testing.T) {
	t.Parallel()

	conn := &fakeConn{}
	manager := newTestManager(func(context.Context) (Conn, error) {
		return conn, nil
	})

	ctx := testContext(t)

	if err := manager.Listen(ctx, "a"); err != nil {
		t.Fatalf("error while listening: %v", err)
	}

	if err := manager.Close(); err != nil {
		t.Fatalf("error while closing: %v", err)
	}

	if err := manager.Close(); err != nil {
		t.Fatalf("error while closing again: %v", err)
	}

	if !conn.closed {
		t.Fatal("the listen connection was not closed")
	}

	if _, _, err := manager.Next(ctx); !errors.Is(err, ErrNotListening) {
		t.Fatalf("got error %v, expected ErrNotListening", err)
	}
}
