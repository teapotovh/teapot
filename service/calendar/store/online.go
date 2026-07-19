package store

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"

	"git.sr.ht/~bitfehler/brant"
	"git.sr.ht/~bitfehler/brant/database/dialect"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/trace"

	"github.com/teapotovh/teapot/lib/httptrace"
	"github.com/teapotovh/teapot/lib/observability"
	"github.com/teapotovh/teapot/lib/pgcache"
	"github.com/teapotovh/teapot/lib/run"
	"github.com/teapotovh/teapot/lib/s3cache"

	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed migrations/*.sql
var migraitions embed.FS

type Online struct {
	logger *slog.Logger

	pool           *pgxpool.Pool
	calendarTable  *pgcache.Table[Path, Calendar]
	objectRefTable *pgcache.Table[Path, objectRef]

	httpTrace     *httptrace.HTTPTrace
	s3endpoint    string
	s3credentials *credentials.Credentials
	s3Secure      bool
	s3Region      string
	s3CacheConfig s3cache.S3CacheConfig
	objectCache   *s3cache.S3Cache

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

	// Use context.Background() here, as we want the pool to live for the lifetime
	// of the program, while the provided context is only meant for databse initialization.
	pool, err := pgxpool.New(context.Background(), psql)
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
		listObjectRefPSQL,
		getObjectRefPSQL,
		storeObjectRefPSQL,
		deleteObjectRefPSQL,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("error while bulding cachnig table: %w", err)
	}

	u, err := url.Parse(s3.URL)
	if err != nil {
		return nil, fmt.Errorf("error while parsing the S3 connection string: %w", err)
	}

	httpTrace := httptrace.NewHTTPTrace()

	key := u.User.Username()
	secret, _ := u.User.Password()
	credentials := credentials.NewStaticV4(key, secret, "")

	p := Online{
		logger: logger,

		pool:           pool,
		calendarTable:  calendarTable,
		objectRefTable: objectTable,

		httpTrace:     httpTrace,
		s3endpoint:    u.Host,
		s3credentials: credentials,
		s3Secure:      u.Scheme == "https",
		s3Region:      s3.Region,
		s3CacheConfig: s3.Cache,
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
func (o *Online) Ping(ctx context.Context) error {
	if err := o.pool.Ping(ctx); err != nil {
		return fmt.Errorf("error while pinging psql: %w", err)
	}

	return nil
}

// Run implements run.Runnable.
func (o *Online) Run(ctx context.Context, notify run.Notify) (err error) {
	client, err := minio.New(o.s3endpoint, &minio.Options{
		Creds:     o.s3credentials,
		Secure:    o.s3Secure,
		Region:    o.s3Region,
		Transport: o.httpTrace.Transport(http.DefaultTransport),
	})
	if err != nil {
		return fmt.Errorf("error while creating the S3 client: %w", err)
	}

	o.objectCache, err = s3cache.NewS3Cache(o.s3CacheConfig, client, o.logger)
	if err != nil {
		return fmt.Errorf("error while building s3 cache: %w", err)
	}

	return run.Combine(o.calendarTable, o.objectRefTable).Run(ctx, notify)
}

// WithTracing implements observability.Tracing.
func (o *Online) WithTracing(tp trace.TracerProvider, tracer trace.Tracer) {
	o.httpTrace.WithTracing(tp, tracer)
}

// Metrics implements observability.Metrics.
func (o *Online) Metrics() []prometheus.Collector {
	return append(o.calendarTable.Metrics(), o.metrics.backend)
}

// ReadinessChecks implements run.ReadinessChecks.
func (o *Online) ReadinessChecks() map[string]observability.Check {
	return o.calendarTable.ReadinessChecks()
}
