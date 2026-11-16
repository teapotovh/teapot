package imap_test

import (
	"context"
	"fmt"
	"net"
	"slices"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/teapotovh/teapot/lib/ldap"
	mailservice "github.com/teapotovh/teapot/service/mail"
	mailimap "github.com/teapotovh/teapot/service/mail/imap"
	"github.com/teapotovh/teapot/service/mail/mailtest"
	"github.com/teapotovh/teapot/service/mail/store"
)

const (
	testUser     = "alice"
	testPassword = "secret"
	testMessage1 = "From: alice@example.org\r\nTo: bob@example.org\r\nSubject: hello\r\n\r\nhello body\r\n"
	testMessage2 = "From: carol@example.org\r\nTo: alice@example.org\r\nSubject: other\r\n\r\nother body\r\n"
)

type testAuth struct{}

func (testAuth) Authenticate(_ context.Context, username, password string) (*ldap.User, error) {
	if username != testUser || password != testPassword {
		return nil, ldap.ErrInvalidCredentials
	}

	return &ldap.User{Username: testUser, Mail: "alice@example.org"}, nil
}

type testNotify struct{}

func (testNotify) Notify() {}

func newTestIMAPServer(t *testing.T) (*mailimap.Server, *store.Mem) {
	t.Helper()

	mem := store.NewMem()
	logger := testLogger()
	listener := mailtest.NewPipeListener()

	mailService, err := mailservice.NewMail(mailservice.Config{
		Store:         store.StoreConfig{Type: "mem"},
		StoreInstance: mem,
		Authenticator: testAuth{},
		Resolver:      testResolver{},
	}, logger)
	if err != nil {
		t.Fatalf("could not create mail service: %v", err)
	}

	srv, err := mailimap.NewServer(mailimap.ServerConfig{
		Network:      "pipe",
		InsecureAuth: true,
		Listener:     listener,
	}, mailService, logger)
	if err != nil {
		t.Fatalf("could not create IMAP server: %v", err)
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
				t.Errorf("IMAP server stopped with error: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("IMAP server did not stop")
		}
	})

	return srv, mem
}

func dialClient(t *testing.T, srv *mailimap.Server) (*imapclient.Client, net.Conn) {
	t.Helper()

	listener, ok := srv.Listener().(*mailtest.PipeListener)
	if !ok {
		t.Fatal("unexpected listener type")
	}

	conn, err := listener.Dial()
	if err != nil {
		t.Fatalf("could not connect to IMAP server: %v", err)
	}

	client := imapclient.New(conn, nil)

	t.Cleanup(func() {
		_ = client.Close()
		_ = conn.Close()
	})

	if err := client.Login(testUser, testPassword).Wait(); err != nil {
		t.Fatalf("could not log in: %v", err)
	}

	return client, conn
}

func appendMessage(t *testing.T, client *imapclient.Client, mailbox, body string) *imap.AppendData {
	t.Helper()

	cmd := client.Append(mailbox, int64(len(body)), nil)
	if _, err := cmd.Write([]byte(body)); err != nil {
		t.Fatalf("could not write append data: %v", err)
	}

	if err := cmd.Close(); err != nil {
		t.Fatalf("could not close append: %v", err)
	}

	data, err := cmd.Wait()
	if err != nil {
		t.Fatalf("append failed: %v", err)
	}

	return data
}

func TestIMAPAppendSelectFetchSearch(t *testing.T) {
	t.Parallel()

	srv, _ := newTestIMAPServer(t)
	client, _ := dialClient(t, srv)

	first := appendMessage(t, client, store.InboxName, testMessage1)
	second := appendMessage(t, client, store.InboxName, testMessage2)

	selected, err := client.Select(store.InboxName, nil).Wait()
	if err != nil {
		t.Fatalf("select failed: %v", err)
	}

	if selected.NumMessages != 2 {
		t.Fatalf("expected 2 messages, got %d", selected.NumMessages)
	}

	if selected.UIDValidity == 0 {
		t.Fatal("expected non-zero UIDVALIDITY")
	}

	if first.UID == 0 || second.UID == 0 || first.UID == second.UID {
		t.Fatalf("invalid appended UIDs: %d, %d", first.UID, second.UID)
	}

	msgs, err := client.Fetch(imap.UIDSetNum(second.UID), &imap.FetchOptions{
		UID:         true,
		Flags:       true,
		Envelope:    true,
		RFC822Size:  true,
		BodySection: []*imap.FetchItemBodySection{{}},
	}).Collect()
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}

	if len(msgs) != 1 {
		t.Fatalf("expected 1 fetched message, got %d", len(msgs))
	}

	if msgs[0].Envelope == nil || msgs[0].Envelope.Subject != "other" {
		t.Fatalf("unexpected envelope: %#v", msgs[0].Envelope)
	}

	if got := string(msgs[0].FindBodySection(&imap.FetchItemBodySection{})); got != testMessage2 {
		t.Fatalf("unexpected body: %q", got)
	}

	result, err := client.Search(&imap.SearchCriteria{Body: []string{"hello body"}}, nil).Wait()
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if len(result.AllSeqNums()) != 1 || result.AllSeqNums()[0] != 1 {
		t.Fatalf("expected sequence 1, got %v", result.AllSeqNums())
	}

	all, err := client.List("", "*", nil).Collect()
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}

	if len(all) != 1 || all[0].Mailbox != store.InboxName {
		t.Fatalf("unexpected list result: %#v", all)
	}
}

func TestIMAPStoreCopyMoveExpunge(t *testing.T) {
	t.Parallel()

	srv, _ := newTestIMAPServer(t)
	client, _ := dialClient(t, srv)

	first := appendMessage(t, client, store.InboxName, testMessage1)
	appendMessage(t, client, store.InboxName, testMessage2)

	if _, err := client.Select(store.InboxName, nil).Wait(); err != nil {
		t.Fatalf("select failed: %v", err)
	}

	if err := client.Store(imap.UIDSetNum(first.UID), &imap.StoreFlags{
		Op:    imap.StoreFlagsAdd,
		Flags: []imap.Flag{imap.FlagSeen},
	}, nil).Close(); err != nil {
		t.Fatalf("store failed: %v", err)
	}

	msgs, err := client.Fetch(imap.UIDSetNum(first.UID), &imap.FetchOptions{Flags: true}).Collect()
	if err != nil {
		t.Fatalf("fetch flags failed: %v", err)
	}

	if len(msgs) != 1 || !hasFlag(msgs[0].Flags, imap.FlagSeen) {
		t.Fatalf("expected \\Seen flag, got %#v", msgs)
	}

	if err := client.Create("Archive", nil).Wait(); err != nil {
		t.Fatalf("create failed: %v", err)
	}

	copyData, err := client.Copy(imap.UIDSetNum(first.UID), "Archive").Wait()
	if err != nil {
		t.Fatalf("copy failed: %v", err)
	}

	if copyData.UIDValidity == 0 {
		t.Fatal("expected COPYUID response")
	}

	archived, err := client.Select("Archive", nil).Wait()
	if err != nil {
		t.Fatalf("select archive failed: %v", err)
	}

	if archived.NumMessages != 1 {
		t.Fatalf("expected 1 archived message, got %d", archived.NumMessages)
	}

	if _, err := client.Select(store.InboxName, nil).Wait(); err != nil {
		t.Fatalf("select inbox failed: %v", err)
	}

	moveData, err := client.Move(imap.UIDSetNum(first.UID), "Archive").Wait()
	if err != nil {
		t.Fatalf("move failed: %v", err)
	}

	if moveData == nil {
		t.Fatal("expected move data")
	}

	if err := client.Store(imap.UIDSetNum(2), &imap.StoreFlags{
		Op:    imap.StoreFlagsAdd,
		Flags: []imap.Flag{imap.FlagDeleted},
	}, nil).Close(); err != nil {
		t.Fatalf("mark deleted failed: %v", err)
	}

	expunged, err := client.Expunge().Collect()
	if err != nil {
		t.Fatalf("expunge failed: %v", err)
	}

	if len(expunged) != 1 {
		t.Fatalf("expected 1 expunge, got %v", expunged)
	}
}

func TestIMAPReadOnlyRejectsStore(t *testing.T) {
	t.Parallel()

	srv, _ := newTestIMAPServer(t)
	client, _ := dialClient(t, srv)

	appendMessage(t, client, store.InboxName, testMessage1)

	if _, err := client.Select(store.InboxName, &imap.SelectOptions{ReadOnly: true}).Wait(); err != nil {
		t.Fatalf("examine failed: %v", err)
	}

	err := client.Store(imap.UIDSetNum(1), &imap.StoreFlags{
		Op:    imap.StoreFlagsAdd,
		Flags: []imap.Flag{imap.FlagSeen},
	}, nil).Close()
	if err == nil {
		t.Fatal("expected STORE to fail on read-only mailbox")
	}
}

func TestIMAPRawProtocol(t *testing.T) {
	t.Parallel()

	srv, _ := newTestIMAPServer(t)

	listener, ok := srv.Listener().(*mailtest.PipeListener)
	if !ok {
		t.Fatal("unexpected listener type")
	}

	conn, err := listener.Dial()
	if err != nil {
		t.Fatalf("could not connect: %v", err)
	}
	defer func() { _ = conn.Close() }()

	reader := newLineReader(conn)

	greeting, err := reader.readLine()
	if err != nil {
		t.Fatalf("could not read greeting: %v", err)
	}

	if len(greeting) < 4 || greeting[:2] != "* " {
		t.Fatalf("unexpected greeting: %q", greeting)
	}

	writeLine(t, conn, "a1 CAPABILITY")

	for {
		line, err := reader.readLine()
		if err != nil {
			t.Fatalf("could not read capability: %v", err)
		}

		if len(line) >= 2 && line[:2] == "a1" {
			break
		}
	}

	writeLine(t, conn, fmt.Sprintf("a2 LOGIN %s %s", testUser, testPassword))

	for {
		line, err := reader.readLine()
		if err != nil {
			t.Fatalf("could not read login: %v", err)
		}

		if len(line) >= 2 && line[:2] == "a2" {
			if line[3] != 'O' || line[4] != 'K' {
				t.Fatalf("login failed: %s", line)
			}

			break
		}
	}
}

func hasFlag(flags []imap.Flag, want imap.Flag) bool {
	return slices.Contains(flags, want)
}

type testResolver struct{}

func (testResolver) ResolveRecipient(_ context.Context, _ string) (string, error) {
	return "", mailservice.ErrRecipientNotFound
}
