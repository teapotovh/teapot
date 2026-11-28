//nolint:paralleltest // the integration tests share one database.
package integration

import (
	"context"
	"log/slog"
	"os"
	"testing"

	postgres "github.com/fergusstrange/embedded-postgres/v2"
	"github.com/fergusstrange/embedded-postgres/v2/eptest"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/teapotovh/teapot/service/bottin/store"
)

const (
	testAttrObjectClass   store.AttributeKey = "objectclass"
	testObjectClassPerson string             = "person"
	testObjectClassTop    string             = "top"
)

var (
	testPSQLURL string
	testPSQL    *store.PSQL
	errTestPSQL error
)

func TestMain(m *testing.M) {
	code := eptest.Run(m, func(pg *postgres.EmbeddedPostgres) {
		testPSQLURL = pg.ConnectionURL()
		testPSQL, errTestPSQL = store.NewPSQL(context.Background(), testPSQLURL, slog.New(slog.DiscardHandler))
	})
	os.Exit(code)
}

func newIntegrationPSQL(t *testing.T) *store.PSQL {
	t.Helper()

	require.NoError(t, errTestPSQL)
	require.NotNil(t, testPSQL)

	conn := testPostgresConn(t)

	if _, err := conn.Exec(t.Context(), "TRUNCATE entries"); err != nil {
		t.Fatalf("truncate entries: %v", err)
	}

	return testPSQL
}

func testPostgresConn(t *testing.T) *pgx.Conn {
	t.Helper()

	conn, err := pgx.Connect(t.Context(), testPSQLURL)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, conn.Close(context.Background()))
	})

	return conn
}

func newTestEntry(t *testing.T, rawDN string, attributes store.Attributes) store.Entry {
	t.Helper()

	dn, err := store.ParseDN(rawDN)
	require.NoError(t, err)

	return store.NewEntry(dn, attributes)
}

func mustTestPrefix(t *testing.T, rawDN string) store.Prefix {
	t.Helper()

	dn, err := store.ParseDN(rawDN)
	require.NoError(t, err)

	return dn.Prefix()
}

func storeTestEntry(t *testing.T, p *store.PSQL, entry store.Entry) {
	t.Helper()

	tx, err := p.Begin(t.Context())
	require.NoError(t, err)

	require.NoError(t, tx.Store(t.Context(), entry))
	require.NoError(t, tx.Commit(t.Context()))
}

func TestPSQLIntegrationStoreGet(t *testing.T) {
	p := newIntegrationPSQL(t)
	entry := newTestEntry(t, "uid=alice,ou=people,dc=bauble,dc=test", store.Attributes{
		testAttrObjectClass: {testObjectClassTop, testObjectClassPerson},
		"uid":               {"alice"},
		"description":       {"comma, equals = slash / backslash \\ quote \""},
	})

	storeTestEntry(t, p, entry)

	got, err := p.Get(t.Context(), entry.DN.Prefix())
	require.NoError(t, err)
	assert.Equal(t, entry.DN, got.DN)
	assert.Equal(t, entry.Attributes, got.Attributes)
}

func TestPSQLIntegrationGetNotFound(t *testing.T) {
	p := newIntegrationPSQL(t)
	prefix := mustTestPrefix(t, "uid=missing,ou=people,dc=bauble,dc=test")

	_, err := p.Get(t.Context(), prefix)
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestPSQLIntegrationListAll(t *testing.T) {
	p := newIntegrationPSQL(t)

	entries := []store.Entry{
		newTestEntry(t, "dc=bauble,dc=test", store.Attributes{testAttrObjectClass: {testObjectClassTop}}),
		newTestEntry(t, "ou=groups,dc=bauble,dc=test", store.Attributes{testAttrObjectClass: {"organizationalUnit"}}),
		newTestEntry(t, "ou=people,dc=bauble,dc=test", store.Attributes{testAttrObjectClass: {"organizationalUnit"}}),
		newTestEntry(
			t,
			"uid=alice,ou=people,dc=bauble,dc=test",
			store.Attributes{testAttrObjectClass: {testObjectClassPerson}},
		),
		newTestEntry(
			t,
			"uid=bob,ou=people,dc=bauble,dc=test",
			store.Attributes{testAttrObjectClass: {testObjectClassPerson}},
		),
	}

	for _, entry := range entries {
		storeTestEntry(t, p, entry)
	}

	got, err := p.List(t.Context(), store.Prefix{})
	require.NoError(t, err)
	require.Len(t, got, len(entries))

	for i, entry := range entries {
		assert.Equal(t, entry.DN, got[i].DN)
	}
}

func TestPSQLIntegrationListPrefix(t *testing.T) {
	p := newIntegrationPSQL(t)

	base := newTestEntry(t, "dc=bauble,dc=test", store.Attributes{testAttrObjectClass: {testObjectClassTop}})
	people := newTestEntry(
		t,
		"ou=people,dc=bauble,dc=test",
		store.Attributes{testAttrObjectClass: {"organizationalUnit"}},
	)
	alice := newTestEntry(
		t,
		"uid=alice,ou=people,dc=bauble,dc=test",
		store.Attributes{testAttrObjectClass: {testObjectClassPerson}},
	)
	groups := newTestEntry(
		t,
		"ou=groups,dc=bauble,dc=test",
		store.Attributes{testAttrObjectClass: {"organizationalUnit"}},
	)

	for _, entry := range []store.Entry{base, people, alice, groups} {
		storeTestEntry(t, p, entry)
	}

	got, err := p.List(t.Context(), people.DN.Prefix())
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, people.DN, got[0].DN)
	assert.Equal(t, alice.DN, got[1].DN)
}

func TestPSQLIntegrationUpdate(t *testing.T) {
	p := newIntegrationPSQL(t)
	entry := newTestEntry(t, "uid=alice,ou=people,dc=bauble,dc=test", store.Attributes{
		testAttrObjectClass: {testObjectClassTop, testObjectClassPerson},
		"uid":               {"alice"},
		"description":       {"first"},
	})

	storeTestEntry(t, p, entry)

	updated := store.NewEntry(entry.DN, store.Attributes{
		testAttrObjectClass: {testObjectClassTop, testObjectClassPerson},
		"uid":               {"alice"},
		"description":       {"second"},
	})
	storeTestEntry(t, p, updated)

	got, err := p.Get(t.Context(), entry.DN.Prefix())
	require.NoError(t, err)
	assert.Equal(t, updated.Attributes, got.Attributes)
}

func TestPSQLIntegrationDelete(t *testing.T) {
	p := newIntegrationPSQL(t)
	entry := newTestEntry(
		t,
		"uid=alice,ou=people,dc=bauble,dc=test",
		store.Attributes{testAttrObjectClass: {testObjectClassPerson}},
	)

	storeTestEntry(t, p, entry)

	tx, err := p.Begin(t.Context())
	require.NoError(t, err)
	require.NoError(t, tx.Delete(t.Context(), entry.DN))
	require.NoError(t, tx.Commit(t.Context()))

	_, err = p.Get(t.Context(), entry.DN.Prefix())
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestPSQLIntegrationCommitRollback(t *testing.T) {
	p := newIntegrationPSQL(t)
	committed := newTestEntry(
		t,
		"uid=commit,ou=people,dc=bauble,dc=test",
		store.Attributes{testAttrObjectClass: {testObjectClassPerson}},
	)
	discarded := newTestEntry(
		t,
		"uid=rollback,ou=people,dc=bauble,dc=test",
		store.Attributes{testAttrObjectClass: {testObjectClassPerson}},
	)

	tx, err := p.Begin(t.Context())
	require.NoError(t, err)
	require.NoError(t, tx.Store(t.Context(), committed))
	require.NoError(t, tx.Commit(t.Context()))

	_, err = p.Get(t.Context(), committed.DN.Prefix())
	require.NoError(t, err)

	tx, err = p.Begin(t.Context())
	require.NoError(t, err)
	require.NoError(t, tx.Store(t.Context(), discarded))
	require.NoError(t, tx.Rollback(t.Context()))

	_, err = p.Get(t.Context(), discarded.DN.Prefix())
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestPSQLIntegrationDNCollation(t *testing.T) {
	newIntegrationPSQL(t)
	conn := testPostgresConn(t)

	var collation string

	err := conn.QueryRow(
		t.Context(),
		`SELECT c.collname
		FROM pg_attribute a
		JOIN pg_collation c ON c.oid = a.attcollation
		WHERE a.attrelid = 'entries'::regclass
			AND a.attname = 'dn';`,
	).Scan(&collation)
	require.NoError(t, err)
	assert.Equal(t, "C", collation)
}
