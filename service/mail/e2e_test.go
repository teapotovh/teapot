package mail_test

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/google/uuid"

	"github.com/teapotovh/teapot/lib/ldap"
	mailservice "github.com/teapotovh/teapot/service/mail"
	mailimap "github.com/teapotovh/teapot/service/mail/imap"
	"github.com/teapotovh/teapot/service/mail/lmtp"
	"github.com/teapotovh/teapot/service/mail/mailtest"
	"github.com/teapotovh/teapot/service/mail/store"
)

const (
	e2eUser     = "alice"
	e2ePassword = "secret"
	e2eAddress  = "alice@example.org"
	e2eMessage  = "From: sender@example.org\r\nTo: alice@example.org\r\nSubject: end to end\r\n\r\nhello from lmtp\r\n"
)

type e2eAuth struct {
	user string
	mail string
}

func (a e2eAuth) Authenticate(_ context.Context, username, password string) (*ldap.User, error) {
	if username != a.user || password != e2ePassword {
		return nil, ldap.ErrInvalidCredentials
	}

	return &ldap.User{Username: a.user, Mail: a.mail}, nil
}

type e2eNotify struct{}

func (e2eNotify) Notify() {}

func e2eLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func TestLMTPToIMAPEndToEnd(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger := e2eLogger()

	owner := "e2e-" + uuid.NewString()
	address := owner + "@example.org"

	mem := store.NewMem()

	mailService, err := mailservice.NewMail(mailservice.Config{
		Store:         store.StoreConfig{Type: "mem"},
		StoreInstance: mem,
		Authenticator: e2eAuth{user: owner, mail: address},
		Resolver:      lmtp.StaticResolver{address: owner},
	}, logger)
	if err != nil {
		t.Fatalf("could not create mail service: %v", err)
	}

	imapListener := mailtest.NewPipeListener()

	imapServer, err := mailimap.NewServer(mailimap.ServerConfig{
		Network:      "pipe",
		InsecureAuth: true,
		Listener:     imapListener,
	}, mailService, logger)
	if err != nil {
		t.Fatalf("could not create IMAP server: %v", err)
	}

	lmtpListener := mailtest.NewPipeListener()

	lmtpServer, err := lmtp.NewServer(lmtp.Config{
		Network:         "tcp",
		Listener:        lmtpListener,
		Domain:          "mail.test",
		MaxMessageBytes: 1024 * 1024,
	}, mailService, logger)
	if err != nil {
		t.Fatalf("could not create LMTP server: %v", err)
	}

	imapDone := make(chan error, 1)
	lmtpDone := make(chan error, 1)

	go func() { imapDone <- imapServer.Run(ctx, e2eNotify{}) }()
	go func() { lmtpDone <- lmtpServer.Run(ctx, e2eNotify{}) }()

	t.Cleanup(func() {
		cancel()

		for name, done := range map[string]chan error{"imap": imapDone, "lmtp": lmtpDone} {
			select {
			case err := <-done:
				if err != nil {
					t.Errorf("%s server stopped with error: %v", name, err)
				}
			case <-time.After(5 * time.Second):
				t.Errorf("%s server did not stop", name)
			}
		}
	})

	deadline := time.Now().Add(5 * time.Second)
	for lmtpServer.Addr() == nil {
		if time.Now().After(deadline) {
			t.Fatal("LMTP server did not start")
		}

		time.Sleep(time.Millisecond)
	}

	deliverLMTP(t, lmtpListener, address, e2eMessage)

	conn, err := imapListener.Dial()
	if err != nil {
		t.Fatalf("could not connect to IMAP: %v", err)
	}
	defer func() { _ = conn.Close() }()

	client := imapclient.New(conn, nil)
	defer func() { _ = client.Close() }()

	if err := client.Login(owner, e2ePassword).Wait(); err != nil {
		t.Fatalf("could not log in: %v", err)
	}

	selected, err := client.Select(store.InboxName, nil).Wait()
	if err != nil {
		t.Fatalf("could not select INBOX: %v", err)
	}

	if selected.NumMessages != 1 {
		t.Fatalf("expected 1 message, got %d", selected.NumMessages)
	}

	msgs, err := client.Fetch(imap.UIDSetNum(1), &imap.FetchOptions{
		UID:         true,
		RFC822Size:  true,
		BodySection: []*imap.FetchItemBodySection{{}},
	}).Collect()
	if err != nil {
		t.Fatalf("could not fetch message: %v", err)
	}

	if len(msgs) != 1 {
		t.Fatalf("expected 1 fetched message, got %d", len(msgs))
	}

	if got := string(msgs[0].FindBodySection(&imap.FetchItemBodySection{})); got != e2eMessage {
		t.Fatalf("unexpected body: %q", got)
	}

	result, err := client.Search(&imap.SearchCriteria{Body: []string{"hello from lmtp"}}, nil).Wait()
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if len(result.AllSeqNums()) != 1 {
		t.Fatalf("expected search hit, got %v", result.AllSeqNums())
	}
}

func deliverLMTP(t *testing.T, listener *mailtest.PipeListener, recipient, message string) {
	t.Helper()

	conn, err := listener.Dial()
	if err != nil {
		t.Fatalf("could not connect to LMTP: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatalf("could not set LMTP deadline: %v", err)
	}

	reader := bufio.NewReader(conn)

	if line := readSMTPLine(t, reader); !strings.HasPrefix(line, "220") {
		t.Fatalf("unexpected greeting: %q", line)
	}

	sendSMTPLine(t, conn, "LHLO client")
	readSMTPResponse(t, reader, "250")

	sendSMTPLine(t, conn, "MAIL FROM:<sender@example.org>")
	readSMTPResponse(t, reader, "250")

	sendSMTPLine(t, conn, fmt.Sprintf("RCPT TO:<%s>", recipient))
	readSMTPResponse(t, reader, "250")

	sendSMTPLine(t, conn, "DATA")
	readSMTPResponse(t, reader, "354")

	for line := range strings.SplitSeq(strings.TrimSuffix(message, "\r\n"), "\r\n") {
		sendSMTPLine(t, conn, line)
	}

	sendSMTPLine(t, conn, ".")
	readSMTPResponse(t, reader, "250")

	sendSMTPLine(t, conn, "QUIT")
	readSMTPResponse(t, reader, "221")
}

func sendSMTPLine(t *testing.T, conn net.Conn, line string) {
	t.Helper()

	if _, err := fmt.Fprintf(conn, "%s\r\n", line); err != nil {
		t.Fatalf("could not write SMTP line: %v", err)
	}
}

func readSMTPLine(t *testing.T, reader *bufio.Reader) string {
	t.Helper()

	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("could not read SMTP line: %v", err)
	}

	return strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
}

func readSMTPResponse(t *testing.T, reader *bufio.Reader, code string) {
	t.Helper()

	for {
		line := readSMTPLine(t, reader)
		if !strings.HasPrefix(line, code) {
			t.Fatalf("expected SMTP code %s, got %q", code, line)
		}

		if len(line) < 4 || line[3] == ' ' {
			return
		}
	}
}
