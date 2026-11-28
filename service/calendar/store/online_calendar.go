package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

func parseCalendarRows(rows pgx.Rows) (calendars []Calendar, err error) {
	defer rows.Close()

	for rows.Next() {
		var (
			path        string
			rawMetadata []byte
		)

		if err := rows.Scan(&path, &rawMetadata); err != nil {
			return nil, fmt.Errorf("could not extract two columns from psql list: %w", err)
		}

		var metadata CalendarMetadata
		if err := json.Unmarshal(rawMetadata, &metadata); err != nil {
			return nil, fmt.Errorf("error while decoding JSON calendar metadata field: %w", err)
		}

		calendars = append(calendars, Calendar{
			Path:     Path(path),
			Metadata: metadata,
		})
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("could not read all psql results: %w", err)
	}

	return calendars, nil
}

var (
	listCalendarQuery = `
		SELECT path, metadata
		FROM calendars
		ORDER BY path;
	`

	getCalendarQuery = `
		SELECT path, metadata
		FROM calendars
		WHERE path = $1
		LIMIT 1;
	`

	rangeCalendarQuery = `
		SELECT path, metadata
		FROM calendars
		WHERE path >= $1
			AND path < $2
		ORDER BY path;
	`

	storeCalendarQuery = `
		INSERT INTO calendars (path, metadata)
		VALUES ($1, $2)
		ON CONFLICT (path) DO UPDATE
		SET metadata = EXCLUDED.metadata;
	`
)

// ListCalendars implements Store.
func (o *Online) ListCalendars(ctx context.Context, basePath Path) (calendars []Calendar, err error) {
	operation := metricsOperationRange
	if basePath == "" {
		operation = metricsOperationList
	}

	start := time.Now()
	defer func() { o.metrics.observeOperation(operation, start, err) }()

	if basePath == "" {
		rows, err := o.pool.Query(ctx, listCalendarQuery)
		if err != nil {
			return nil, fmt.Errorf("error while listing calendars from psql: %w", err)
		}

		return parseCalendarRows(rows)
	}

	rows, err := o.pool.Query(ctx, rangeCalendarQuery, basePath.String(), prefixEnd(basePath).String())
	if err != nil {
		return nil, fmt.Errorf("error while listing calendars from psql: %w", err)
	}

	return parseCalendarRows(rows)
}

// GetCalendar implements Store.
func (o *Online) GetCalendar(ctx context.Context, path Path) (calendar *Calendar, err error) {
	start := time.Now()
	defer func() { o.metrics.observeOperation(metricsOperationGet, start, err) }()

	return o.getCalendar(ctx, path)
}

func (o *Online) getCalendar(ctx context.Context, path Path) (*Calendar, error) {
	rows, err := o.pool.Query(ctx, getCalendarQuery, path.String())
	if err != nil {
		return nil, fmt.Errorf("error while getting calendar from psql: %w", err)
	}

	calendars, err := parseCalendarRows(rows)
	if err != nil {
		return nil, err
	}

	if len(calendars) == 0 {
		return nil, ErrCalendarNotFound
	}

	return &calendars[0], nil
}

// PutCalendar implements Store.
func (o *Online) PutCalendar(ctx context.Context, calendar Calendar) (bool, error) {
	_, err := o.getCalendar(ctx, calendar.Path)
	exists := err == nil

	if err != nil && !errors.Is(err, ErrNotFound) {
		return false, err
	}

	err = runInTx(ctx, o, func(ctx context.Context, tx Transaction) error {
		return tx.StoreCalendar(ctx, calendar)
	})
	if err != nil {
		return false, err
	}

	return !exists, nil
}

// StoreCalendar implements Transaction.
func (t *psqlTransaction) StoreCalendar(ctx context.Context, calendar Calendar) (err error) {
	start := time.Now()
	defer func() { t.metrics.observeOperation(metricsOperationStore, start, err) }()

	rawMetadata, err := json.Marshal(calendar.Metadata)
	if err != nil {
		return fmt.Errorf("could not marshal metadata of calendar for psql: %w", err)
	}

	if _, err := t.tx.Exec(ctx, storeCalendarQuery, calendar.Path.String(), rawMetadata); err != nil {
		return fmt.Errorf("error while inserting calendar with psql: %w", err)
	}

	return nil
}
