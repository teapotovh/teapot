package store

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"git.sr.ht/~bitfehler/brant"
	"git.sr.ht/~bitfehler/brant/database/dialect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/teapotovh/teapot/lib/observability"
	"github.com/teapotovh/teapot/lib/pgcache"
	"github.com/teapotovh/teapot/lib/run"

	_ "github.com/jackc/pgx/v5/stdlib"
)

var (
	ErrNotFound = errors.New("not found")
)

func parseRows(rows pgx.Rows) ([]Entry, error) {
	defer rows.Close()

	var entries []Entry
	for rows.Next() {
		var (
			rawPrefix     string
			rawAttributes []byte
		)

		if err := rows.Scan(&rawPrefix, &rawAttributes); err != nil {
			return nil, fmt.Errorf("could not extract two columns from psql list: %w", err)
		}

		var attributes Attributes
		if err := json.Unmarshal(rawAttributes, &attributes); err != nil {
			return nil, fmt.Errorf("error while decoding JSON attributes field: %w", err)
		}

		prefix, err := ParsePrefix(rawPrefix)
		if err != nil {
			return nil, fmt.Errorf("error while decoding entry prefix: %w", err)
		}

		entries = append(entries, Entry{
			DN:         prefix.DN(),
			Attributes: attributes,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("could not read all psql results: %w", err)
	}

	return entries, nil
}

var listQuery = `
		SELECT dn, attributes
		FROM entries;
`

func listPSQL(ctx context.Context, conn *pgxpool.Pool) ([]Entry, error) {
	rows, err := conn.Query(ctx, listQuery)
	if err != nil {
		return nil, fmt.Errorf("error while listing entries from psql: %w", err)
	}

	return parseRows(rows)
}

var getQuery = `
		SELECT dn, attributes
		FROM entries
		WHERE dn = ANY($1);
`

func getPSQL(ctx context.Context, conn *pgxpool.Pool, prefixes []Prefix) ([]Entry, error) {
	dns := make([]string, 0, len(prefixes))
	for _, prefix := range prefixes {
		dns = append(dns, prefix.String())
	}

	rows, err := conn.Query(ctx, getQuery, dns)
	if err != nil {
		return nil, fmt.Errorf("error while listing select entries from psql: %w", err)
	}

	return parseRows(rows)
}

var storeQuery = `
		INSERT INTO entries (dn, attributes)
	  SELECT unnest($1::text[]), unnest($2::jsonb[])
		ON CONFLICT (dn) DO UPDATE
		SET attributes = EXCLUDED.attributes;
`

func storePSQL(ctx context.Context, tx pgx.Tx, entries []Entry) error {
	dns := make([]string, 0, len(entries))
	attrs := make([][]byte, 0, len(entries))

	for i, entry := range entries {
		rawAttributes, err := json.Marshal(entry.Attributes)
		if err != nil {
			return fmt.Errorf("could not marshal attributes of entry %d for psql: %w", i, err)
		}

		prefix := entry.DN.Prefix()
		dns = append(dns, prefix.String())
		attrs = append(attrs, rawAttributes)
	}

	_, err := tx.Exec(ctx, storeQuery, dns, attrs)
	if err != nil {
		return fmt.Errorf("error while inserting entries with psql: %w", err)
	}

	return nil
}

var deleteQuery = `DELETE FROM entries WHERE dn = ANY($1);`

func deletePSQL(ctx context.Context, tx pgx.Tx, prefixes []Prefix) error {
	dns := make([]string, 0, len(prefixes))
	for _, prefix := range prefixes {
		dns = append(dns, prefix.String())
	}

	_, err := tx.Exec(ctx, deleteQuery, dns)
	if err != nil {
		return fmt.Errorf("error while deleting entries in psql: %w", err)
	}

	return nil
}

//go:embed migrations/*.sql
var migraitions embed.FS

type PSQL struct {
	pool  *pgxpool.Pool
	table *pgcache.Table[Prefix, Entry]

	metrics metrics
}

func NewPSQL(ctx context.Context, url string, logger *slog.Logger) (*PSQL, error) {
	options := brant.DefaultOptions().WithTableName("_version").WithFilesystem(migraitions).WithDataSourceName(url)

	provider, err := brant.NewProvider(logger, dialect.Postgres, options)
	if err != nil {
		return nil, fmt.Errorf("error while constructing migration provider: %w", err)
	}

	applied, err := provider.Up(ctx)
	if err != nil {
		return nil, fmt.Errorf("error while applying migrations: %w", err)
	}

	for _, migration := range applied {
		logger.Info("applied migration", "migration", migration)
	}

	if err := provider.Close(); err != nil {
		return nil, fmt.Errorf("error while closing migration connection: %w", err)
	}

	// Use context.Background() here, as we want the pool to live for the lifetime
	// of the program, while the provided context is only meant for databse initialization.
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		return nil, fmt.Errorf("error while opening connection pool to psql: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("error while connecting to psql: %w", err)
	}

	table, err := pgcache.NewTable(pool, "entries", PrefixFromString, listPSQL, getPSQL, storePSQL, deletePSQL, logger)
	if err != nil {
		return nil, fmt.Errorf("error while bulding cachnig table: %w", err)
	}

	p := PSQL{
		pool:  pool,
		table: table,
	}
	p.metrics.initMetrics("psql")

	return &p, nil
}

// Ping implements Store.
func (p *PSQL) Ping(ctx context.Context) error {
	if err := p.pool.Ping(ctx); err != nil {
		return fmt.Errorf("error while pinging psql: %w", err)
	}

	return nil
}

// List implements Store.
func (p *PSQL) List(ctx context.Context, prefix Prefix, exact bool) (entries []Entry, _ error) {
	if exact {
		entry, found := p.table.Get(prefix)
		if found {
			entries = append(entries, entry)
		}
	} else {
		end := prefixEnd(prefix)
		entries = slices.Collect(p.table.Between(prefix, end))
	}

	return entries, nil
}

// Begin implements Store.
func (p *PSQL) Begin(ctx context.Context) (Transaction, error) {
	tx, err := p.table.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not begin pgcache transaction: %w", err)
	}

	return &PSQLTransaction{tx: tx}, nil
}

type PSQLTransaction struct {
	tx *pgcache.TableTx[Prefix, Entry]
}

// Store implements Transaction.
func (p *PSQLTransaction) Store(ctx context.Context, entry Entry) (err error) {
	return p.tx.Store(ctx, []Entry{entry})
}

// Delete implements Transaction.
func (p *PSQLTransaction) Delete(ctx context.Context, dn DN) (err error) {
	prefix := dn.Prefix()
	return p.tx.Delete(ctx, []Prefix{prefix})
}

// Commit implements Transaction.
func (p *PSQLTransaction) Commit(ctx context.Context) (err error) {
	return p.tx.Commit(ctx)
}

// Run implements run.Runnable.
func (p *PSQL) Run(ctx context.Context, notify run.Notify) error {
	return p.table.Run(ctx, notify)
}

// Metrics implements observability.Metrics.
func (p *PSQL) Metrics() []prometheus.Collector {
	return append(p.table.Metrics(), p.metrics.backend)
}

// ReadinessChecks implements run.ReadinessChecks.
func (p *PSQL) ReadinessChecks() map[string]observability.Check {
	return p.table.ReadinessChecks()
}
