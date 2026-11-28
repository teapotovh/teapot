package store

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"git.sr.ht/~bitfehler/brant"
	"git.sr.ht/~bitfehler/brant/database/dialect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/trace"

	"github.com/teapotovh/teapot/lib/httptrace"
	"github.com/teapotovh/teapot/lib/observability"
	"github.com/teapotovh/teapot/lib/run"
	"github.com/teapotovh/teapot/lib/s3cache"

	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed migrations/*.sql
var migraitions embed.FS

// psqlPool is the subset of the psql pool used by the store.
type psqlPool interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Begin(ctx context.Context) (pgx.Tx, error)
	Ping(ctx context.Context) error
	Close()
}

// Transaction provides access to one calendar store transaction.
type Transaction interface {
	// StoreCalendar inserts or updates a calendar.
	StoreCalendar(ctx context.Context, calendar Calendar) error

	// StoreObjectRef inserts or updates a calendar object reference.
	StoreObjectRef(ctx context.Context, ref ObjectRef) error

	// DeleteObjectRef removes a calendar object reference.
	DeleteObjectRef(ctx context.Context, path Path) error

	// Commit saves the changes made through this transaction.
	Commit(ctx context.Context) error

	// Rollback discards the changes made through this transaction.
	Rollback(ctx context.Context) error
}

type psqlTransaction struct {
	tx      pgx.Tx
	metrics *metrics
}

type Online struct {
	logger *slog.Logger

	pool psqlPool

	httpTrace     *httptrace.HTTPTrace
	s3URL         string
	s3Region      string
	s3CacheConfig s3cache.S3CacheConfig
	objectCache   s3cache.Blob

	metrics metrics
}

func NewOnline(ctx context.Context, psql string, s3 StoreS3Config, logger *slog.Logger) (*Online, error) {
	options := brant.DefaultOptions().WithTableName("_version").WithFilesystem(migraitions).WithDataSourceName(psql)

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

	config, err := pgxpool.ParseConfig(psql)
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

	httpTrace := httptrace.NewHTTPTrace()

	p := Online{
		logger: logger,

		pool: pool,

		httpTrace:     httpTrace,
		s3URL:         s3.URL,
		s3Region:      s3.Region,
		s3CacheConfig: s3.Cache,
	}
	p.metrics.initMetrics("psql")

	return &p, nil
}

// Begin starts a transaction over the calendar store tables.
func (o *Online) Begin(ctx context.Context) (result Transaction, err error) {
	start := time.Now()
	defer func() { o.metrics.observeOperation(metricsOperationBegin, start, err) }()

	tx, err := o.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not begin psql transaction: %w", err)
	}

	return &psqlTransaction{tx: tx, metrics: &o.metrics}, nil
}

func rollbackOnError(ctx context.Context, tx Transaction, err *error) {
	if *err == nil {
		return
	}

	*err = errors.Join(*err, tx.Rollback(ctx))
}

func runInTx(ctx context.Context, o *Online, fn func(context.Context, Transaction) error) (err error) {
	tx, err := o.Begin(ctx)
	if err != nil {
		return err
	}

	defer rollbackOnError(ctx, tx, &err)

	if err := fn(ctx, tx); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// Commit implements Transaction.
func (t *psqlTransaction) Commit(ctx context.Context) (err error) {
	start := time.Now()
	defer func() { t.metrics.observeOperation(metricsOperationCommit, start, err) }()

	if err := t.tx.Commit(ctx); err != nil {
		return fmt.Errorf("error while committing transaction: %w", err)
	}

	return nil
}

// Rollback implements Transaction.
func (t *psqlTransaction) Rollback(ctx context.Context) (err error) {
	start := time.Now()
	defer func() { t.metrics.observeOperation(metricsOperationRollback, start, err) }()

	if err := t.tx.Rollback(ctx); err != nil {
		return fmt.Errorf("error while rolling back transaction: %w", err)
	}

	return nil
}

// Ping implements Store.
func (o *Online) Ping(ctx context.Context) error {
	if err := o.pool.Ping(ctx); err != nil {
		return fmt.Errorf("error while pinging psql: %w", err)
	}

	return nil
}

// Run implements run.Runnable.
func (o *Online) Run(step run.Step) (err error) {
	ctx := step.Context()

	defer o.pool.Close()

	client, err := s3cache.NewClient(o.s3URL, o.s3Region, o.httpTrace.Transport(http.DefaultTransport))
	if err != nil {
		return fmt.Errorf("error while creating the S3 client: %w", err)
	}

	o.objectCache, err = s3cache.NewS3Cache(o.s3CacheConfig, client, o.logger)
	if err != nil {
		return fmt.Errorf("error while building s3 cache: %w", err)
	}

	step.Notify()

	<-ctx.Done()

	return nil
}

// WithTracing implements observability.Tracing.
func (o *Online) WithTracing(tp trace.TracerProvider, tracer trace.Tracer) {
	o.httpTrace.WithTracing(tp, tracer)
}

// ReadinessChecks implements run.ReadinessChecks.
func (o *Online) ReadinessChecks() map[string]observability.Check {
	return map[string]observability.Check{
		"calendar/store/psql": observability.CheckFunc(o.Ping),
	}
}

// Ensure *Online implements Store.
var _ Store = (*Online)(nil)

// Ensure *psqlTransaction implements Transaction.
var _ Transaction = (*psqlTransaction)(nil)

// Ensure *pgxpool.Pool implements psqlPool.
var _ psqlPool = (*pgxpool.Pool)(nil)
