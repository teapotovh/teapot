package store

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errPSQLTest = errors.New("psql test error")

const (
	testAttrObjectClass   AttributeKey = "objectclass"
	testObjectClassPerson string       = "person"
	testObjectClassTop    string       = "top"
)

func newMockPSQL(t *testing.T) (*PSQL, pgxmock.PgxPoolIface) {
	t.Helper()

	pool, err := pgxmock.NewPool(pgxmock.QueryMatcherOption(pgxmock.QueryMatcherEqual))
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	p := PSQL{pool: pool}
	p.metrics.initMetrics("test")

	return &p, pool
}

func mustTestPrefix(t *testing.T, rawDN string) Prefix {
	t.Helper()

	dn, err := ParseDN(rawDN)
	require.NoError(t, err)

	return dn.Prefix()
}

func testEntryRows(t *testing.T, prefix Prefix, attributes Attributes) *pgxmock.Rows {
	t.Helper()

	rawAttributes, err := json.Marshal(attributes)
	require.NoError(t, err)

	return pgxmock.NewRows([]string{"dn", "attributes"}).AddRow(prefix.String(), rawAttributes)
}

func beginTestTransaction(t *testing.T, p *PSQL, pool pgxmock.PgxPoolIface) Transaction {
	t.Helper()

	pool.ExpectBegin()

	tx, err := p.Begin(t.Context())
	require.NoError(t, err)

	return tx
}

func TestPSQLGet(t *testing.T) {
	t.Parallel()

	p, pool := newMockPSQL(t)
	prefix := mustTestPrefix(t, "uid=alice,ou=people,dc=bauble,dc=test")
	attributes := Attributes{
		testAttrObjectClass: {testObjectClassTop, testObjectClassPerson},
		"uid":               {"alice"},
	}

	pool.ExpectQuery(getQuery).
		WithArgs(prefix.String()).
		WillReturnRows(testEntryRows(t, prefix, attributes))

	entry, err := p.Get(t.Context(), prefix)
	require.NoError(t, err)
	assert.Equal(t, prefix.DN(), entry.DN)
	assert.Equal(t, attributes, entry.Attributes)
	require.NoError(t, pool.ExpectationsWereMet())
}

func TestPSQLGetNotFound(t *testing.T) {
	t.Parallel()

	p, pool := newMockPSQL(t)
	prefix := mustTestPrefix(t, "uid=missing,ou=people,dc=bauble,dc=test")

	pool.ExpectQuery(getQuery).
		WithArgs(prefix.String()).
		WillReturnRows(pgxmock.NewRows([]string{"dn", "attributes"}))

	_, err := p.Get(t.Context(), prefix)
	require.ErrorIs(t, err, ErrNotFound)
	require.NoError(t, pool.ExpectationsWereMet())
}

func TestPSQLListAll(t *testing.T) {
	t.Parallel()

	p, pool := newMockPSQL(t)
	base := mustTestPrefix(t, "dc=bauble,dc=test")
	people := mustTestPrefix(t, "ou=people,dc=bauble,dc=test")
	alice := mustTestPrefix(t, "uid=alice,ou=people,dc=bauble,dc=test")

	rows := pgxmock.NewRows([]string{"dn", "attributes"}).
		AddRow(base.String(), []byte(`{"objectclass":["top"]}`)).
		AddRow(people.String(), []byte(`{"objectclass":["organizationalUnit"]}`)).
		AddRow(alice.String(), []byte(`{"objectclass":["person"],"uid":["alice"]}`))

	pool.ExpectQuery(listQuery).WillReturnRows(rows)

	entries, err := p.List(t.Context(), Prefix{})
	require.NoError(t, err)
	require.Len(t, entries, 3)
	assert.Equal(t, base.DN(), entries[0].DN)
	assert.Equal(t, people.DN(), entries[1].DN)
	assert.Equal(t, alice.DN(), entries[2].DN)
	require.NoError(t, pool.ExpectationsWereMet())
}

func TestPSQLListPrefix(t *testing.T) {
	t.Parallel()

	p, pool := newMockPSQL(t)
	base := mustTestPrefix(t, "ou=people,dc=bauble,dc=test")
	alice := mustTestPrefix(t, "uid=alice,ou=people,dc=bauble,dc=test")

	rows := pgxmock.NewRows([]string{"dn", "attributes"}).
		AddRow(base.String(), []byte(`{"objectclass":["organizationalUnit"]}`)).
		AddRow(alice.String(), []byte(`{"objectclass":["person"],"uid":["alice"]}`))

	pool.ExpectQuery(rangeQuery).
		WithArgs(base.String(), prefixEnd(base).String()).
		WillReturnRows(rows)

	entries, err := p.List(t.Context(), base)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.Equal(t, base.DN(), entries[0].DN)
	assert.Equal(t, alice.DN(), entries[1].DN)
	require.NoError(t, pool.ExpectationsWereMet())
}

func TestPSQLDecodeErrors(t *testing.T) {
	t.Parallel()

	t.Run("json", func(t *testing.T) {
		t.Parallel()

		p, pool := newMockPSQL(t)
		prefix := mustTestPrefix(t, "uid=alice,ou=people,dc=bauble,dc=test")

		pool.ExpectQuery(getQuery).
			WithArgs(prefix.String()).
			WillReturnRows(pgxmock.NewRows([]string{"dn", "attributes"}).
				AddRow(prefix.String(), []byte(`{`)))

		_, err := p.Get(t.Context(), prefix)
		require.ErrorContains(t, err, "error while decoding JSON attributes field")
		require.NoError(t, pool.ExpectationsWereMet())
	})

	t.Run("scan", func(t *testing.T) {
		t.Parallel()

		p, pool := newMockPSQL(t)
		prefix := mustTestPrefix(t, "uid=alice,ou=people,dc=bauble,dc=test")

		pool.ExpectQuery(getQuery).
			WithArgs(prefix.String()).
			WillReturnRows(pgxmock.NewRows([]string{"dn"}).AddRow(prefix.String()))

		_, err := p.Get(t.Context(), prefix)
		require.ErrorContains(t, err, "could not extract two columns from psql list")
		require.NoError(t, pool.ExpectationsWereMet())
	})

	t.Run("prefix", func(t *testing.T) {
		t.Parallel()

		p, pool := newMockPSQL(t)
		prefix := mustTestPrefix(t, "uid=alice,ou=people,dc=bauble,dc=test")

		pool.ExpectQuery(getQuery).
			WithArgs(prefix.String()).
			WillReturnRows(pgxmock.NewRows([]string{"dn", "attributes"}).
				AddRow("not-a-dn", []byte(`{"objectclass":["top"]}`)))

		_, err := p.Get(t.Context(), prefix)
		require.ErrorContains(t, err, "error while decoding entry prefix")
		require.NoError(t, pool.ExpectationsWereMet())
	})
}

func TestPSQLGetQueryError(t *testing.T) {
	t.Parallel()

	p, pool := newMockPSQL(t)
	prefix := mustTestPrefix(t, "uid=alice,ou=people,dc=bauble,dc=test")

	pool.ExpectQuery(getQuery).
		WithArgs(prefix.String()).
		WillReturnError(errPSQLTest)

	_, err := p.Get(t.Context(), prefix)
	require.ErrorIs(t, err, errPSQLTest)
	require.ErrorContains(t, err, "error while getting entry from psql")
	require.NoError(t, pool.ExpectationsWereMet())
}

func TestPSQLListQueryError(t *testing.T) {
	t.Parallel()

	p, pool := newMockPSQL(t)
	prefix := mustTestPrefix(t, "ou=people,dc=bauble,dc=test")

	pool.ExpectQuery(rangeQuery).
		WithArgs(prefix.String(), prefixEnd(prefix).String()).
		WillReturnError(errPSQLTest)

	_, err := p.List(t.Context(), prefix)
	require.ErrorIs(t, err, errPSQLTest)
	require.ErrorContains(t, err, "error while listing entries from psql")
	require.NoError(t, pool.ExpectationsWereMet())
}

func TestPSQLBeginError(t *testing.T) {
	t.Parallel()

	p, pool := newMockPSQL(t)

	pool.ExpectBegin().WillReturnError(errPSQLTest)

	_, err := p.Begin(t.Context())
	require.ErrorIs(t, err, errPSQLTest)
	require.ErrorContains(t, err, "could not begin psql transaction")
	require.NoError(t, pool.ExpectationsWereMet())
}

func TestPSQLStore(t *testing.T) {
	t.Parallel()

	p, pool := newMockPSQL(t)
	dn, err := ParseDN("uid=alice,ou=people,dc=bauble,dc=test")
	require.NoError(t, err)

	entry := NewEntry(dn, Attributes{testAttrObjectClass: {testObjectClassTop, testObjectClassPerson}})
	rawAttributes, err := json.Marshal(entry.Attributes)
	require.NoError(t, err)

	tx := beginTestTransaction(t, p, pool)

	pool.ExpectExec(storeQuery).
		WithArgs(entry.DN.Prefix().String(), rawAttributes).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	pool.ExpectCommit()

	require.NoError(t, tx.Store(t.Context(), entry))
	require.NoError(t, tx.Commit(t.Context()))
	require.NoError(t, pool.ExpectationsWereMet())
}

func TestPSQLStoreError(t *testing.T) {
	t.Parallel()

	p, pool := newMockPSQL(t)
	dn, err := ParseDN("uid=alice,ou=people,dc=bauble,dc=test")
	require.NoError(t, err)

	entry := NewEntry(dn, Attributes{testAttrObjectClass: {testObjectClassTop}})
	rawAttributes, err := json.Marshal(entry.Attributes)
	require.NoError(t, err)

	tx := beginTestTransaction(t, p, pool)

	pool.ExpectExec(storeQuery).
		WithArgs(entry.DN.Prefix().String(), rawAttributes).
		WillReturnError(errPSQLTest)

	err = tx.Store(t.Context(), entry)
	require.ErrorIs(t, err, errPSQLTest)
	require.ErrorContains(t, err, "error while inserting entries with psql")
	require.NoError(t, pool.ExpectationsWereMet())
}

func TestPSQLDelete(t *testing.T) {
	t.Parallel()

	p, pool := newMockPSQL(t)
	dn, err := ParseDN("uid=alice,ou=people,dc=bauble,dc=test")
	require.NoError(t, err)

	tx := beginTestTransaction(t, p, pool)

	pool.ExpectExec(deleteQuery).
		WithArgs(dn.Prefix().String()).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))
	pool.ExpectCommit()

	require.NoError(t, tx.Delete(t.Context(), dn))
	require.NoError(t, tx.Commit(t.Context()))
	require.NoError(t, pool.ExpectationsWereMet())
}

func TestPSQLDeleteError(t *testing.T) {
	t.Parallel()

	p, pool := newMockPSQL(t)
	dn, err := ParseDN("uid=alice,ou=people,dc=bauble,dc=test")
	require.NoError(t, err)

	tx := beginTestTransaction(t, p, pool)

	pool.ExpectExec(deleteQuery).
		WithArgs(dn.Prefix().String()).
		WillReturnError(errPSQLTest)

	err = tx.Delete(t.Context(), dn)
	require.ErrorIs(t, err, errPSQLTest)
	require.ErrorContains(t, err, "error while deleting entries in psql")
	require.NoError(t, pool.ExpectationsWereMet())
}

func TestPSQLCommitError(t *testing.T) {
	t.Parallel()

	p, pool := newMockPSQL(t)
	tx := beginTestTransaction(t, p, pool)

	pool.ExpectCommit().WillReturnError(errPSQLTest)

	err := tx.Commit(t.Context())
	require.ErrorIs(t, err, errPSQLTest)
	require.ErrorContains(t, err, "error while committing transaction")
	require.NoError(t, pool.ExpectationsWereMet())
}

func TestPSQLRollback(t *testing.T) {
	t.Parallel()

	p, pool := newMockPSQL(t)
	tx := beginTestTransaction(t, p, pool)

	pool.ExpectRollback()

	require.NoError(t, tx.Rollback(t.Context()))
	require.NoError(t, pool.ExpectationsWereMet())
}

func TestPSQLRollbackError(t *testing.T) {
	t.Parallel()

	p, pool := newMockPSQL(t)
	tx := beginTestTransaction(t, p, pool)

	pool.ExpectRollback().WillReturnError(errPSQLTest)

	err := tx.Rollback(t.Context())
	require.ErrorIs(t, err, errPSQLTest)
	require.ErrorContains(t, err, "error while rolling back transaction")
	require.NoError(t, pool.ExpectationsWereMet())
}

func TestPSQLPing(t *testing.T) {
	t.Parallel()

	p, pool := newMockPSQL(t)

	pool.ExpectPing().WillReturnError(errPSQLTest)

	err := p.Ping(t.Context())
	require.ErrorIs(t, err, errPSQLTest)
	require.ErrorContains(t, err, "error while pinging psql")
	require.NoError(t, pool.ExpectationsWereMet())
}
