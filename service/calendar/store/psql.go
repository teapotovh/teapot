package store

import (
	"context"
	"embed"
	"fmt"
	"log/slog"

	"git.sr.ht/~bitfehler/brant"
	"git.sr.ht/~bitfehler/brant/database/dialect"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/teapotovh/teapot/lib/observability"
	"github.com/teapotovh/teapot/lib/pgcache"
	"github.com/teapotovh/teapot/lib/run"

	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed migrations/*.sql
var migraitions embed.FS

type PSQL struct {
	pool          *pgxpool.Pool
	calendarTable *pgcache.Table[Path, Calendar]
	objectTable   *pgcache.Table[Path, Object]

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

	calendarTable, err := pgcache.NewTable(
		pool,
		"calendars",
		PrefixFromString,
		listCalendarPSQL,
		getCalendarPSQL,
		storeCalendarPSQL,
		deleteCalendarPSQL,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("error while bulding cachnig table: %w", err)
	}

	objectTable, err := pgcache.NewTable(
		pool,
		"objects",
		PrefixFromString,
		listObjectPSQL,
		getObjectPSQL,
		storeObjectPSQL,
		deleteObjectPSQL,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("error while bulding cachnig table: %w", err)
	}

	p := PSQL{
		pool:          pool,
		calendarTable: calendarTable,
		objectTable:   objectTable,
	}
	p.metrics.initMetrics("psql")

	return &p, nil
}

// unit may be used with runInTx when no result is expected.
type unit struct{}

func runInTx[T pgcache.Object[Path], R any](
	table *pgcache.Table[Path, T],
	fn func(ctx context.Context, tx *pgcache.TableTx[Path, T]) (R, error),
) func(context.Context) (R, error) {
	var empty R

	return func(ctx context.Context) (R, error) {
		tx, err := table.Begin(ctx)
		if err != nil {
			return empty, fmt.Errorf("error while starting transaction: %w", err)
		}

		result, err := fn(ctx, tx)
		if err != nil {
			return empty, fmt.Errorf("error while running transaction body: %w", err)
		}

		if err := tx.Commit(ctx); err != nil {
			return empty, fmt.Errorf("error while committing transaction: %w", err)
		}

		return result, nil
	}
}

// Ping implements Store.
func (p *PSQL) Ping(ctx context.Context) error {
	if err := p.pool.Ping(ctx); err != nil {
		return fmt.Errorf("error while pinging psql: %w", err)
	}

	return nil
}

// Run implements run.Runnable.
func (p *PSQL) Run(ctx context.Context, notify run.Notify) error {
	cr := run.Combine(p.calendarTable, p.objectTable)
	return cr.Run(ctx, notify)
}

// Metrics implements observability.Metrics.
func (p *PSQL) Metrics() []prometheus.Collector {
	return append(p.calendarTable.Metrics(), p.metrics.backend)
}

// ReadinessChecks implements run.ReadinessChecks.
func (p *PSQL) ReadinessChecks() map[string]observability.Check {
	return p.calendarTable.ReadinessChecks()
}
