package store

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"git.sr.ht/~bitfehler/brant"
	"git.sr.ht/~bitfehler/brant/database/dialect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/teapotovh/teapot/lib/observability"

	_ "github.com/jackc/pgx/v5/stdlib"
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

var (
	listQuery = `
		SELECT dn, attributes
		FROM entries
		ORDER BY dn;
	`

	getQuery = `
		SELECT dn, attributes
		FROM entries
		WHERE dn = $1
		LIMIT 1;
	`

	rangeQuery = `
		SELECT dn, attributes
		FROM entries
		WHERE dn >= $1
			AND dn < $2
		ORDER BY dn;
	`

	storeQuery = `
		INSERT INTO entries (dn, attributes)
		VALUES ($1, $2)
		ON CONFLICT (dn) DO UPDATE
		SET attributes = EXCLUDED.attributes;
	`

	deleteQuery = `DELETE FROM entries WHERE dn = $1;`
)

//go:embed migrations/*.sql
var migraitions embed.FS

type psqlPool interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Begin(ctx context.Context) (pgx.Tx, error)
	Ping(ctx context.Context) error
}

type PSQL struct {
	pool psqlPool

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

	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("error while parsing psql connection URL: %w", err)
	}

	config.MaxConns = 2
	config.MinConns = 1

	// Use context.Background() here, as we want the pool to live for the lifetime
	// of the program, while the provided context is only meant for databse initialization.
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		return nil, fmt.Errorf("error while opening connection pool to psql: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("error while connecting to psql: %w", err)
	}

	p := PSQL{
		pool: pool,
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

// Get implements Store.
func (p *PSQL) Get(ctx context.Context, prefix Prefix) (entry Entry, err error) {
	start := time.Now()
	defer func() { p.metrics.observeOperation(metricsOperationGet, start, err) }()

	rows, err := p.pool.Query(ctx, getQuery, prefix.String())
	if err != nil {
		return Entry{}, fmt.Errorf("error while getting entry from psql: %w", err)
	}

	entries, err := parseRows(rows)
	if err != nil {
		return Entry{}, err
	}

	if len(entries) == 0 {
		return Entry{}, ErrNotFound
	}

	return entries[0], nil
}

// List implements Store.
func (p *PSQL) List(ctx context.Context, prefix Prefix) (entries []Entry, err error) {
	operation := metricsOperationRange
	if len(prefix) == 0 {
		operation = metricsOperationList
	}

	start := time.Now()
	defer func() { p.metrics.observeOperation(operation, start, err) }()

	if len(prefix) == 0 {
		rows, err := p.pool.Query(ctx, listQuery)
		if err != nil {
			return nil, fmt.Errorf("error while listing entries from psql: %w", err)
		}

		return parseRows(rows)
	}

	rows, err := p.pool.Query(ctx, rangeQuery, prefix.String(), prefixEnd(prefix).String())
	if err != nil {
		return nil, fmt.Errorf("error while listing entries from psql: %w", err)
	}

	return parseRows(rows)
}

// Begin implements Store.
func (p *PSQL) Begin(ctx context.Context) (result Transaction, err error) {
	start := time.Now()
	defer func() { p.metrics.observeOperation(metricsOperationBegin, start, err) }()

	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not begin psql transaction: %w", err)
	}

	return &PSQLTransaction{tx: tx, metrics: &p.metrics}, nil
}

type PSQLTransaction struct {
	tx      pgx.Tx
	metrics *metrics
}

// Store implements Transaction.
func (p *PSQLTransaction) Store(ctx context.Context, entry Entry) (err error) {
	start := time.Now()
	defer func() { p.metrics.observeOperation(metricsOperationStore, start, err) }()

	rawAttributes, err := json.Marshal(entry.Attributes)
	if err != nil {
		return fmt.Errorf("could not marshal attributes of entry for psql: %w", err)
	}

	prefix := entry.DN.Prefix()
	if _, err := p.tx.Exec(ctx, storeQuery, prefix.String(), rawAttributes); err != nil {
		return fmt.Errorf("error while inserting entries with psql: %w", err)
	}

	return nil
}

// Delete implements Transaction.
func (p *PSQLTransaction) Delete(ctx context.Context, dn DN) (err error) {
	start := time.Now()
	defer func() { p.metrics.observeOperation(metricsOperationDelete, start, err) }()

	prefix := dn.Prefix()
	if _, err := p.tx.Exec(ctx, deleteQuery, prefix.String()); err != nil {
		return fmt.Errorf("error while deleting entries in psql: %w", err)
	}

	return nil
}

// Commit implements Transaction.
func (p *PSQLTransaction) Commit(ctx context.Context) (err error) {
	start := time.Now()
	defer func() { p.metrics.observeOperation(metricsOperationCommit, start, err) }()

	if err := p.tx.Commit(ctx); err != nil {
		return fmt.Errorf("error while committing transaction: %w", err)
	}

	return nil
}

// Rollback implements Transaction.
func (p *PSQLTransaction) Rollback(ctx context.Context) (err error) {
	start := time.Now()
	defer func() { p.metrics.observeOperation(metricsOperationRollback, start, err) }()

	if err := p.tx.Rollback(ctx); err != nil {
		return fmt.Errorf("error while rolling back transaction: %w", err)
	}

	return nil
}

// ReadinessChecks implements observability.ReadinessChecks.
func (p *PSQL) ReadinessChecks() map[string]observability.Check {
	return map[string]observability.Check{
		"bottin/store/psql": observability.CheckFunc(p.Ping),
	}
}

// Ensure *PSQL implements Store.
var _ Store = (*PSQL)(nil)

// Ensure *PSQLTransaction implements Transaction.
var _ Transaction = (*PSQLTransaction)(nil)
