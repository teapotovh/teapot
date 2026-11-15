package lmtp

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/teapotovh/teapot/lib/ldap"
	mailservice "github.com/teapotovh/teapot/service/mail"
	"github.com/teapotovh/teapot/service/mail/mailtest"
	"github.com/teapotovh/teapot/service/mail/store"
)

const (
	testDomain    = "mail.test"
	testTimeout   = 5 * time.Second
	testWaitDelay = time.Millisecond

	aliceAddress = "alice@mail.test"
	aliceOwner   = "alice"
	bobAddress   = "bob@mail.test"
	bobOwner     = "bob"

	testFrom = "sender@mail.test"
)

type testNotify struct{}

func (testNotify) Notify() {}

type testClient struct {
	t      *testing.T
	conn   net.Conn
	reader *bufio.Reader
	writer *bufio.Writer
}

func newTestServer(t *testing.T, maxMessageBytes int64) (*Server, *store.Mem) {
	t.Helper()

	mem := store.NewMem()

	return newTestServerStore(t, maxMessageBytes, mem), mem
}

func newTestServerStore(t *testing.T, maxMessageBytes int64, mailStore store.Store) *Server {
	t.Helper()

	logger := slog.New(slog.DiscardHandler)
	listener := mailtest.NewPipeListener()

	mailService, err := mailservice.NewMail(mailservice.Config{
		Store:         store.StoreConfig{Type: "mem"},
		StoreInstance: mailStore,
		Authenticator: testAuth{},
		Resolver: StaticResolver{
			aliceAddress: aliceOwner,
			bobAddress:   bobOwner,
		},
	}, logger)
	if err != nil {
		t.Fatalf("could not create mail service: %v", err)
	}

	srv, err := NewServer(Config{
		Network:         "tcp",
		Listener:        listener,
		Domain:          testDomain,
		MaxMessageBytes: maxMessageBytes,
		ReadTimeout:     testTimeout,
		WriteTimeout:    testTimeout,
	}, mailService, logger)
	if err != nil {
		t.Fatalf("could not create server: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() {
		done <- srv.Run(ctx, testNotify{})
	}()

	t.Cleanup(func() {
		cancel()

		select {
		case err := <-done:
			if err != nil {
				t.Errorf("server stopped with error: %v", err)
			}
		case <-time.After(testTimeout):
			t.Error("server did not stop before timeout")
		}

		_ = srv.Close()
	})

	deadline := time.Now().Add(testTimeout)
	for srv.Addr() == nil {
		if time.Now().After(deadline) {
			t.Fatal("server did not start listening before timeout")
		}

		time.Sleep(testWaitDelay)
	}

	return srv
}

func newTestClient(t *testing.T, srv *Server) *testClient {
	t.Helper()

	listener, ok := srv.Listener().(*mailtest.PipeListener)
	if !ok {
		t.Fatal("unexpected listener type")
	}

	conn, err := listener.Dial()
	if err != nil {
		t.Fatalf("could not connect to server: %v", err)
	}

	if err := conn.SetDeadline(time.Now().Add(testTimeout)); err != nil {
		t.Fatalf("could not set connection deadline: %v", err)
	}

	t.Cleanup(func() {
		_ = conn.Close()
	})

	return &testClient{
		t:      t,
		conn:   conn,
		reader: bufio.NewReader(conn),
		writer: bufio.NewWriter(conn),
	}
}

func (c *testClient) send(format string, args ...any) {
	c.t.Helper()

	if _, err := fmt.Fprintf(c.writer, format+"\r\n", args...); err != nil {
		c.t.Fatalf("could not write command: %v", err)
	}

	if err := c.writer.Flush(); err != nil {
		c.t.Fatalf("could not flush command: %v", err)
	}
}

func (c *testClient) reply() []string {
	c.t.Helper()

	line, err := c.reader.ReadString('\n')
	if err != nil {
		c.t.Fatalf("could not read response: %v", err)
	}

	line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
	lines := []string{line}

	for len(line) >= 4 && line[3] == '-' {
		line, err = c.reader.ReadString('\n')
		if err != nil {
			c.t.Fatalf("could not read response continuation: %v", err)
		}

		line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		lines = append(lines, line)
	}

	return lines
}

func (c *testClient) expectCode(code string) []string {
	c.t.Helper()

	lines := c.reply()
	if len(lines[0]) < 4 || !strings.HasPrefix(lines[0], code) || (lines[0][3] != ' ' && lines[0][3] != '-') {
		c.t.Fatalf("expected response code %s, got %q", code, lines[0])
	}

	return lines
}

func (c *testClient) greet() {
	c.t.Helper()
	c.expectCode("220")
}

func (c *testClient) lhlo() {
	c.t.Helper()

	c.send("LHLO client")
	c.expectCode("250")
}

func inboxMessages(t *testing.T, mem *store.Mem, owner string) []store.Message {
	t.Helper()

	mailbox, err := mem.GetMailbox(context.Background(), owner, store.InboxName)
	if errors.Is(err, store.ErrMailboxNotFound) {
		return nil
	}

	if err != nil {
		t.Fatalf("could not get inbox for %q: %v", owner, err)
	}

	messages, err := mem.ListMessages(context.Background(), mailbox.ID)
	if err != nil {
		t.Fatalf("could not list messages for %q: %v", owner, err)
	}

	return messages
}

func messageBody(t *testing.T, mem *store.Mem, message store.Message) []byte {
	t.Helper()

	body, err := mem.Body(context.Background(), message)
	if err != nil {
		t.Fatalf("could not read message body: %v", err)
	}

	return body
}

func TestLMTPDelivery(t *testing.T) {
	t.Parallel()

	srv, mem := newTestServer(t, 1024)
	c := newTestClient(t, srv)

	c.greet()
	c.lhlo()

	c.send("MAIL FROM:<%s>", testFrom)
	c.expectCode("250")

	c.send("RCPT TO:<%s>", aliceAddress)
	c.expectCode("250")

	c.send("DATA")
	c.expectCode("354")

	body := "Subject: test\r\n\r\nhello\r\n"
	c.send("%s.", body)

	lines := c.expectCode("250")
	if !strings.Contains(lines[0], aliceAddress) {
		t.Fatalf("expected per-recipient success for %q, got %q", aliceAddress, lines[0])
	}

	messages := inboxMessages(t, mem, aliceOwner)
	if len(messages) != 1 {
		t.Fatalf("expected 1 message in inbox, got %d", len(messages))
	}

	if got := string(messageBody(t, mem, messages[0])); got != body {
		t.Fatalf("unexpected message body: got %q, want %q", got, body)
	}
}

func TestLMTPUnknownRecipient(t *testing.T) {
	t.Parallel()

	srv, mem := newTestServer(t, 1024)
	c := newTestClient(t, srv)

	c.greet()
	c.lhlo()

	c.send("MAIL FROM:<%s>", testFrom)
	c.expectCode("250")

	c.send("RCPT TO:<nobody@mail.test>")
	c.expectCode("550")

	if messages := inboxMessages(t, mem, aliceOwner); len(messages) != 0 {
		t.Fatalf("expected no messages in inbox, got %d", len(messages))
	}
}

func TestLMTPMultipleRecipients(t *testing.T) {
	t.Parallel()

	srv, mem := newTestServer(t, 1024)
	c := newTestClient(t, srv)

	c.greet()
	c.lhlo()

	c.send("MAIL FROM:<%s>", testFrom)
	c.expectCode("250")

	c.send("RCPT TO:<%s>", aliceAddress)
	c.expectCode("250")

	c.send("RCPT TO:<%s>", bobAddress)
	c.expectCode("250")

	c.send("DATA")
	c.expectCode("354")

	body := "Subject: hello\r\n\r\nmessage\r\n"
	c.send("%s.", body)

	aliceReply := c.expectCode("250")
	if !strings.Contains(aliceReply[0], aliceAddress) {
		t.Fatalf("expected success for %q, got %q", aliceAddress, aliceReply[0])
	}

	bobReply := c.expectCode("250")
	if !strings.Contains(bobReply[0], bobAddress) {
		t.Fatalf("expected success for %q, got %q", bobAddress, bobReply[0])
	}

	for _, owner := range []string{aliceOwner, bobOwner} {
		messages := inboxMessages(t, mem, owner)
		if len(messages) != 1 {
			t.Fatalf("expected 1 message in %q inbox, got %d", owner, len(messages))
		}

		if got := string(messageBody(t, mem, messages[0])); got != body {
			t.Fatalf("unexpected body in %q inbox: got %q, want %q", owner, got, body)
		}
	}
}

func TestLMTPMessageTooLarge(t *testing.T) {
	t.Parallel()

	srv, mem := newTestServer(t, 16)
	c := newTestClient(t, srv)

	c.greet()
	c.lhlo()

	c.send("MAIL FROM:<%s>", testFrom)
	c.expectCode("250")

	c.send("RCPT TO:<%s>", aliceAddress)
	c.expectCode("250")

	c.send("DATA")
	c.expectCode("354")

	c.send("Subject: a message that is too large\r\n\r\nbody\r\n.")
	c.expectCode("552")

	if messages := inboxMessages(t, mem, aliceOwner); len(messages) != 0 {
		t.Fatalf("expected no messages in inbox, got %d", len(messages))
	}
}

var errInjectedAppend = errors.New("injected append failure")

type failingAppendStore struct {
	store.Store

	calls atomic.Int32
}

func (s *failingAppendStore) Append(
	ctx context.Context,
	mailboxID uuid.UUID,
	data []byte,
	flags []string,
	internalDate time.Time,
) (*store.Message, error) {
	if s.calls.Add(1) == 2 {
		return nil, errInjectedAppend
	}

	return s.Store.Append(ctx, mailboxID, data, flags, internalDate)
}

func TestLMTPPerRecipientFailure(t *testing.T) {
	t.Parallel()

	mem := store.NewMem()
	failing := &failingAppendStore{Store: mem}
	srv := newTestServerStore(t, 1024, failing)

	c := newTestClient(t, srv)
	c.greet()
	c.lhlo()

	c.send("MAIL FROM:<%s>", testFrom)
	c.expectCode("250")

	c.send("RCPT TO:<%s>", aliceAddress)
	c.expectCode("250")

	c.send("RCPT TO:<%s>", bobAddress)
	c.expectCode("250")

	c.send("DATA")
	c.expectCode("354")

	c.send("Subject: partial\r\n\r\nbody\r\n.")
	c.expectCode("250")
	c.expectCode("451")
}

type testAuth struct{}

func (testAuth) Authenticate(_ context.Context, _, _ string) (*ldap.User, error) {
	return &ldap.User{Username: aliceOwner}, nil
}
