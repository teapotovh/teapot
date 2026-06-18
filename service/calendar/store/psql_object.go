package store

import (
	"context"
	"fmt"
	"slices"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/teapotovh/teapot/lib/pgcache"
)

func parseObjectRows(rows pgx.Rows) (objects []Object, err error) {
	defer rows.Close()

	for rows.Next() {
		var obj Object

		if err := rows.Scan(&obj.Path, &obj.ModTime, &obj.Data); err != nil {
			return nil, fmt.Errorf("could not extract three columns from psql list: %w", err)
		}

		objects = append(objects, obj)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("could not read all psql results: %w", err)
	}

	return objects, nil
}

var listObjectQuery = `
		SELECT path, mod_time, data
		FROM objects;
`

func listObjectPSQL(ctx context.Context, conn *pgxpool.Pool) ([]Object, error) {
	rows, err := conn.Query(ctx, listObjectQuery)
	if err != nil {
		return nil, fmt.Errorf("error while listing objects from psql: %w", err)
	}

	return parseObjectRows(rows)
}

var getObjectQuery = `
		SELECT path, mod_time, data
		FROM objects
		WHERE path = ANY($1);
`

func getObjectPSQL(ctx context.Context, conn *pgxpool.Pool, paths []Path) ([]Object, error) {
	ps := make([]string, 0, len(paths))
	for _, path := range paths {
		ps = append(ps, path.String())
	}

	rows, err := conn.Query(ctx, getObjectQuery, ps)
	if err != nil {
		return nil, fmt.Errorf("error while listing select objects from psql: %w", err)
	}

	return parseObjectRows(rows)
}

var storeObjectQuery = `
		INSERT INTO objects (path, mod_time, data)
		SELECT unnest($1::text[]), unnest($2::timestamptz[]), unnest($3::bytea[])
		ON CONFLICT (path) DO UPDATE
		SET mod_time = EXCLUDED.mod_time, data = EXCLUDED.data;
`

func storeObjectPSQL(ctx context.Context, tx pgx.Tx, objects []Object) error {
	paths := make([]string, 0, len(objects))
	modTimes := make([]time.Time, 0, len(objects))
	datas := make([][]byte, 0, len(objects))

	for _, object := range objects {
		paths = append(paths, string(object.Path))
		modTimes = append(modTimes, object.ModTime)
		datas = append(datas, object.Data)
	}

	_, err := tx.Exec(ctx, storeObjectQuery, paths, modTimes, datas)
	if err != nil {
		return fmt.Errorf("error while inserting objects with psql: %w", err)
	}

	return nil
}

var deleteObjectQuery = `DELETE FROM objects WHERE path = ANY($1);`

func deleteObjectPSQL(ctx context.Context, tx pgx.Tx, paths []Path) error {
	ps := make([]string, 0, len(paths))
	for _, path := range paths {
		ps = append(ps, path.String())
	}

	_, err := tx.Exec(ctx, deleteObjectQuery, ps)
	if err != nil {
		return fmt.Errorf("error while deleting objects in psql: %w", err)
	}

	return nil
}

// CreateCalendarObject implements Store.
func (p *PSQL) CreateCalendarObject(ctx context.Context, object Object) error {
	_, err := runInTx(p.objectTable, func(ctx context.Context, tx *pgcache.TableTx[Path, Object]) (unit, error) {
		return unit{}, tx.Store(ctx, []Object{object})
	})(ctx)
	return err
}

// ListCalendarObjects implements Store.
func (p *PSQL) ListCalendarObjects(ctx context.Context, basePath Path) ([]Object, error) {
	endPath := Path(fmt.Sprintf("%s%c", basePath, utf8.MaxRune))
	iter := p.objectTable.Between(basePath, endPath)
	return slices.Collect(iter), nil
}

// GetCalendarObject implements Store.
func (p *PSQL) GetCalendarObject(ctx context.Context, path Path) (*Object, error) {
	object, found := p.objectTable.Get(path)
	if !found {
		return nil, ErrNotFound
	}
	return &object, nil
}

// DeleteCalendarObject implements Store.
func (p *PSQL) DeleteCalendarObject(ctx context.Context, path Path) error {
	_, err := runInTx(p.objectTable, func(ctx context.Context, tx *pgcache.TableTx[Path, Object]) (unit, error) {
		return unit{}, tx.Delete(ctx, []Path{path})
	})(ctx)
	return err
}
