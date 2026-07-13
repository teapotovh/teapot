package store

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/teapotovh/teapot/lib/pgcache"
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

var listCalendarQuery = `
		SELECT path, metadata
		FROM calendars;
`

func listCalendarPSQL(ctx context.Context, conn *pgxpool.Pool) ([]Calendar, error) {
	rows, err := conn.Query(ctx, listCalendarQuery)
	if err != nil {
		return nil, fmt.Errorf("error while listing calendars from psql: %w", err)
	}

	return parseCalendarRows(rows)
}

var getCalendarQuery = `
		SELECT path, metadata
		FROM calendars
		WHERE path = ANY($1);
`

func getCalendarPSQL(ctx context.Context, conn *pgxpool.Pool, paths []Path) ([]Calendar, error) {
	ps := make([]string, 0, len(paths))
	for _, path := range paths {
		ps = append(ps, path.String())
	}

	rows, err := conn.Query(ctx, getCalendarQuery, ps)
	if err != nil {
		return nil, fmt.Errorf("error while listing select calendars from psql: %w", err)
	}

	return parseCalendarRows(rows)
}

var storeCalendarQuery = `
		INSERT INTO calendars (path, metadata)
		SELECT unnest($1::text[]), unnest($2::jsonb[])
		ON CONFLICT (path) DO UPDATE
		SET metadata = EXCLUDED.metadata
`

func storeCalendarPSQL(ctx context.Context, tx pgx.Tx, calendars []Calendar) error {
	paths := make([]string, 0, len(calendars))
	metadata := make([][]byte, 0, len(calendars))

	for i, calendar := range calendars {
		rawMetadata, err := json.Marshal(calendar.Metadata)
		if err != nil {
			return fmt.Errorf("could not marshal metadata of calendar %d for psql: %w", i, err)
		}

		paths = append(paths, string(calendar.Path))
		metadata = append(metadata, rawMetadata)
	}

	_, err := tx.Exec(ctx, storeCalendarQuery, paths, metadata)
	if err != nil {
		return fmt.Errorf("error while inserting calendars with psql: %w", err)
	}

	return nil
}

var deleteCalendarQuery = `DELETE FROM calendars WHERE path = ANY($1);`

func deleteCalendarPSQL(ctx context.Context, tx pgx.Tx, paths []Path) error {
	ps := make([]string, 0, len(paths))
	for _, path := range paths {
		ps = append(ps, path.String())
	}

	_, err := tx.Exec(ctx, deleteCalendarQuery, ps)
	if err != nil {
		return fmt.Errorf("error while deleting calendars in psql: %w", err)
	}

	return nil
}

// CreateCalendar implements Store.
func (o *Online) CreateCalendar(ctx context.Context, calendar Calendar) error {
	if _, exists := o.calendarTable.Get(calendar.Path); exists {
		return ErrAlreadyExists
	}

	_, err := runInTx(o.calendarTable, func(ctx context.Context, tx *pgcache.TableTx[Path, Calendar]) (unit, error) {
		return unit{}, tx.Store(ctx, []Calendar{calendar})
	})(ctx)

	return err
}

// ListCalendars implements Store.
func (o *Online) ListCalendars(ctx context.Context, basePath Path) ([]Calendar, error) {
	endPath := Path(fmt.Sprintf("%s%c", basePath, utf8.MaxRune))
	iter := o.calendarTable.Between(basePath, endPath)

	return slices.Collect(iter), nil
}

// GetCalendar implements Store.
func (o *Online) GetCalendar(ctx context.Context, path Path) (*Calendar, error) {
	calendar, found := o.calendarTable.Get(path)
	if !found {
		return nil, ErrCalendarNotFound
	}

	return &calendar, nil
}
