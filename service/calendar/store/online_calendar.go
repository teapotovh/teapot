package store

import (
	"context"
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

var listCalendarQuery = `
		SELECT path, name, description
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
		SELECT path, name, description
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
		INSERT INTO calendars (path, name, description)
		SELECT unnest($1::text[]), unnest($2::text[]), unnest($3::text[])
		ON CONFLICT (path) DO UPDATE
		SET name = EXCLUDED.name, description = EXCLUDED.description;
`

func storeCalendarPSQL(ctx context.Context, tx pgx.Tx, calendars []Calendar) error {
	paths := make([]string, 0, len(calendars))
	names := make([]string, 0, len(calendars))
	descriptions := make([]string, 0, len(calendars))

	for _, calendar := range calendars {
		paths = append(paths, string(calendar.Path))
		names = append(names, calendar.Name)
		descriptions = append(descriptions, calendar.Description)
	}

	_, err := tx.Exec(ctx, storeCalendarQuery, paths, names, descriptions)
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
