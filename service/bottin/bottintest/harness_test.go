package bottintest

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/go-ldap/ldap/v3"
	"github.com/jsimonetti/pwscheme/ssha512"
	"github.com/stretchr/testify/require"

	"github.com/teapotovh/teapot/lib/ldapsrv"
	"github.com/teapotovh/teapot/lib/run"
	"github.com/teapotovh/teapot/service/bottin"
	"github.com/teapotovh/teapot/service/bottin/store"
)

const (
	baseDN     = "dc=bauble,dc=test"
	rootPasswd = "root-secret"

	peopleDN = "ou=people," + baseDN
	groupsDN = "ou=groups," + baseDN

	aliceDN = "uid=alice," + peopleDN
	bobDN   = "uid=bob," + peopleDN

	alicePasswd = "alice-secret"
	bobPasswd   = "bob-secret"
)

type testServer struct {
	addr   string
	bottin *bottin.Bottin
}

type testStep struct {
	ctx    context.Context
	notify func()
}

func (s testStep) Context() context.Context { return s.ctx }

func (s testStep) Notify() { s.notify() }

func (testStep) Add(string, run.Runnable) {}

func (testStep) Wait() {}

func noopStep() testStep {
	return testStep{ctx: context.Background(), notify: func() {}}
}

func readyStep(ctx context.Context, ready func()) testStep {
	return testStep{ctx: ctx, notify: ready}
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()

	return newTestServerWithACL(t, []string{"*::bind read add modify delete compare:*:*"})
}

func newTestServerWithACL(t *testing.T, acl []string) *testServer {
	t.Helper()

	logger := slog.New(slog.DiscardHandler)

	server, err := bottin.NewBottin(bottin.BottinConfig{
		Store:  store.StoreConfig{Type: "mem"},
		BaseDN: baseDN,
		Passwd: rootPasswd,
		ACL:    acl,
	}, logger)
	require.NoError(t, err)

	require.NoError(t, server.Run(noopStep()))
	seed(t, server)

	ldapServer, err := ldapsrv.NewServer[bottin.State](ldapsrv.LDAPSrvConfig{
		Address:       "127.0.0.1:0",
		ShutdownDelay: time.Second,
	}, logger)
	require.NoError(t, err)

	ldapServer.Register(server)

	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan struct{})
	done := make(chan error, 1)

	var readyOnce sync.Once

	go func() {
		done <- ldapServer.Run(readyStep(ctx, func() {
			readyOnce.Do(func() {
				close(ready)
			})
		}))
	}()

	t.Cleanup(func() {
		cancel()

		select {
		case err := <-done:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Logf("ldap server stopped with an error: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Error("ldap server did not shut down within two seconds")
		}
	})

	select {
	case <-ready:
	case err := <-done:
		require.NoError(t, err, "ldap server stopped before becoming ready")
	case <-time.After(5 * time.Second):
		t.Fatal("ldap server did not become ready within five seconds")
	}

	addr := ldapServer.Addr()
	require.NotNil(t, addr)

	return &testServer{addr: addr.String(), bottin: server}
}

func seed(t *testing.T, server *bottin.Bottin) {
	t.Helper()

	aliceHash, err := ssha512.Generate(alicePasswd, 16)
	require.NoError(t, err)
	bobHash, err := ssha512.Generate(bobPasswd, 16)
	require.NoError(t, err)

	entries := []store.Entry{
		store.NewEntry(mustParseDN(t, peopleDN), store.Attributes{
			store.NewAttributeKey("objectclass"): store.AttributeValue{objectClassTop, "organizationalUnit"},
		}),
		store.NewEntry(mustParseDN(t, groupsDN), store.Attributes{
			store.NewAttributeKey("objectclass"): store.AttributeValue{objectClassTop, "organizationalUnit"},
		}),
		store.NewEntry(mustParseDN(t, aliceDN), store.Attributes{
			store.NewAttributeKey("objectclass"): store.AttributeValue{
				objectClassTop,
				"person",
				"organizationalPerson",
				"inetOrgPerson",
			},
			store.NewAttributeKey("cn"):           store.AttributeValue{"Alice Anderson"},
			store.NewAttributeKey("sn"):           store.AttributeValue{"Anderson"},
			store.NewAttributeKey("uid"):          store.AttributeValue{"alice"},
			store.NewAttributeKey("mail"):         store.AttributeValue{"alice@example.com"},
			store.NewAttributeKey("userpassword"): store.AttributeValue{aliceHash},
			store.NewAttributeKey("memberof"):     store.AttributeValue{"cn=admins," + groupsDN},
		}),
		store.NewEntry(mustParseDN(t, bobDN), store.Attributes{
			store.NewAttributeKey("objectclass"): store.AttributeValue{
				objectClassTop,
				"person",
				"organizationalPerson",
				"inetOrgPerson",
			},
			store.NewAttributeKey("cn"):           store.AttributeValue{"Bob Brown"},
			store.NewAttributeKey("sn"):           store.AttributeValue{"Brown"},
			store.NewAttributeKey("uid"):          store.AttributeValue{"bob"},
			store.NewAttributeKey("mail"):         store.AttributeValue{"bob@example.com"},
			store.NewAttributeKey("userpassword"): store.AttributeValue{bobHash},
		}),
		store.NewEntry(mustParseDN(t, "cn=admins,"+groupsDN), store.Attributes{
			store.NewAttributeKey("objectclass"): store.AttributeValue{objectClassTop, "groupOfNames"},
			store.NewAttributeKey("member"):      store.AttributeValue{aliceDN},
		}),
	}

	ctx := context.Background()

	tx, err := server.Store().Begin(ctx)
	require.NoError(t, err)

	for _, entry := range entries {
		require.NoError(t, tx.Store(ctx, entry))
	}

	require.NoError(t, tx.Commit(ctx))
}

func mustParseDN(t *testing.T, raw string) store.DN {
	t.Helper()

	dn, err := store.ParseDN(raw)
	require.NoError(t, err)

	return dn
}

func (s *testServer) dial(t *testing.T) *ldap.Conn {
	t.Helper()

	conn, err := ldap.DialURL("ldap://" + s.addr)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = conn.Close()
	})

	return conn
}

func (s *testServer) dialRoot(t *testing.T) *ldap.Conn {
	t.Helper()

	conn := s.dial(t)
	require.NoError(t, conn.Bind(baseDN, rootPasswd))

	return conn
}

func (s *testServer) dialAlice(t *testing.T) *ldap.Conn {
	t.Helper()

	conn := s.dial(t)
	require.NoError(t, conn.Bind(aliceDN, alicePasswd))

	return conn
}

func search(t *testing.T, conn *ldap.Conn, base string, scope int, filter string, attrs []string) *ldap.SearchResult {
	t.Helper()

	result, err := conn.Search(ldap.NewSearchRequest(
		base,
		scope,
		ldap.NeverDerefAliases,
		0,
		0,
		false,
		filter,
		attrs,
		nil,
	))
	require.NoError(t, err)

	return result
}

func requireLDAPError(t *testing.T, err error, code uint16) {
	t.Helper()

	require.Error(t, err)

	var ldapErr *ldap.Error
	require.ErrorAs(t, err, &ldapErr)
	require.Equal(t, code, ldapErr.ResultCode, "unexpected LDAP result code: %v", err)
}
