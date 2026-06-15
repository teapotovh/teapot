package store

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"time"

	"git.sr.ht/~bitfehler/brant"
	"git.sr.ht/~bitfehler/brant/database/dialect"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed migrations/*.sql
var migraitions embed.FS

type PSQL struct {
	pool *pgxpool.Pool

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

	// Use context.Background() here, as we want the pool to live for the lifetime
	// of the program, while the provided context is only meant for databse initialization.
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		return nil, fmt.Errorf("error while opening connection pool to psql: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("error while connecting to psql: %w", err)
	}

	p := PSQL{pool: pool}
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

var storeCalendarQuery = `
	INSERT INTO calendars (path, name, description)
	VALUES ($1, $2, $3);
`

// CreateCalendar implements Store.
func (p *PSQL) CreateCalendar(ctx context.Context, calendar Calendar) (err error) {
	start := time.Now()

	defer func() {
		p.metrics.operationDuration.WithLabelValues(operationCreateCalendar, status(err)).
			Observe(time.Since(start).Seconds())
	}()

	_, err = p.pool.Exec(ctx, storeCalendarQuery, calendar.Path, calendar.Name, calendar.Description)
	if err != nil {
		return fmt.Errorf("error while inserting data with psql: %w", err)
	}

	return nil
}

var listCalendarsQuery = `
	SELECT path, name, description
	FROM calendars
	WHERE path LIKE $1;
`

// ListCalendars implements Store.
func (p *PSQL) ListCalendars(ctx context.Context, basePath string) (calendars []Calendar, err error) {
	start := time.Now()

	defer func() {
		p.metrics.operationDuration.WithLabelValues(operationListCalendars, status(err)).
			Observe(time.Since(start).Seconds())
	}()

	rows, err := p.pool.Query(ctx, listCalendarsQuery, basePath+"%")
	if err != nil {
		return nil, fmt.Errorf("error while listing calendars from psql: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var cal Calendar

		if err := rows.Scan(&cal.Path, &cal.Name, &cal.Description); err != nil {
			return nil, fmt.Errorf("could not extract three columns from psql list: %w", err)
		}

		calendars = append(calendars, cal)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("could not read all psql results: %w", err)
	}

	return calendars, nil
}

var getCalendarQuery = `
	SELECT path, name, description
	FROM calendars
	WHERE path = $1
	LIMIT 1;
`

// GetCalendar implements Store.
func (p *PSQL) GetCalendar(ctx context.Context, path string) (calendar *Calendar, err error) {
	start := time.Now()

	defer func() {
		p.metrics.operationDuration.WithLabelValues(operationGetCalendar, status(err)).
			Observe(time.Since(start).Seconds())
	}()

	row := p.pool.QueryRow(ctx, getCalendarQuery, path)

	var cal Calendar
	if err := row.Scan(&cal.Path, &cal.Description); err != nil {
		return nil, fmt.Errorf("could not extract three columns from row entry: %w", err)
	}

	return &cal, nil
}

// CreateCalendarObject implements Store.
func (p *PSQL) CreateCalendarObject(ctx, object Object) error {
	return nil
}

// ListCalendarObjects implements Store.
func (p *PSQL) ListCalendarObjects(ctx, path string) ([]Object, error) {
	return nil, nil
}

// GetCalendarObject implements Store.
func (p *PSQL) GetCalendarObject(ctx context.Context, path string) (*Object, error) {
	return nil, nil
}

// DeleteCalendarObject implements Store.
func (p *PSQL) DeleteCalendarObject(ctx, path string) error {
	return nil
}

// Metrics implements observability.Metrics.
func (p *PSQL) Metrics() []prometheus.Collector {
	return []prometheus.Collector{
		p.metrics.backend,
		p.metrics.operationDuration,
		p.metrics.transactionDuration,
	}
}
