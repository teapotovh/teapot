package store

import (
	"context"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/teapotovh/teapot/lib/pgcache"
	"github.com/teapotovh/teapot/lib/s3cache"
)

type objectRef struct {
	Path    Path
	ModTime time.Time

	Ref  uuid.UUID
	ETag s3cache.Hash
}

// Key implements pgcache.Object.
func (ref objectRef) Key() Path {
	return ref.Path
}

func (ref objectRef) s3Key() string {
	return ref.Ref.String()
}

func parseObjectRows(rows pgx.Rows) (refs []objectRef, err error) {
	defer rows.Close()

	for rows.Next() {
		var ref objectRef

		if err := rows.Scan(&ref.Path, &ref.ModTime, &ref.Ref, &ref.ETag); err != nil {
			return nil, fmt.Errorf("could not extract three columns from psql list: %w", err)
		}

		refs = append(refs, ref)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("could not read all psql results: %w", err)
	}

	return refs, nil
}

var listObjectQuery = `
		SELECT path, mod_time, ref, etag
		FROM object_refs;
`

func listObjectRefPSQL(ctx context.Context, conn *pgxpool.Pool) ([]objectRef, error) {
	rows, err := conn.Query(ctx, listObjectQuery)
	if err != nil {
		return nil, fmt.Errorf("error while listing object refs from psql: %w", err)
	}

	return parseObjectRows(rows)
}

var getObjectQuery = `
		SELECT path, mod_time, ref, etag
		FROM object_refs
		WHERE path = ANY($1);
`

func getObjectRefPSQL(ctx context.Context, conn *pgxpool.Pool, paths []Path) ([]objectRef, error) {
	ps := make([]string, 0, len(paths))
	for _, path := range paths {
		ps = append(ps, path.String())
	}

	rows, err := conn.Query(ctx, getObjectQuery, ps)
	if err != nil {
		return nil, fmt.Errorf("error while listing select object refs from psql: %w", err)
	}

	return parseObjectRows(rows)
}

var storeObjectQuery = `
		INSERT INTO object_refs (path, mod_time, ref, etag)
		SELECT unnest($1::text[]), unnest($2::timestamptz[]), unnest($3::uuid[]), unnest($4::text[])
		ON CONFLICT (path) DO UPDATE
		SET mod_time = EXCLUDED.mod_time, etag = EXCLUDED.etag;
`

func storeObjectRefPSQL(ctx context.Context, tx pgx.Tx, refs []objectRef) error {
	paths := make([]string, 0, len(refs))
	modTimes := make([]time.Time, 0, len(refs))
	uuids := make([]uuid.UUID, 0, len(refs))
	etags := make([]string, 0, len(refs))

	for _, ref := range refs {
		paths = append(paths, string(ref.Path))
		modTimes = append(modTimes, ref.ModTime)
		uuids = append(uuids, ref.Ref)
		etags = append(etags, ref.ETag.String())
	}

	_, err := tx.Exec(ctx, storeObjectQuery, paths, modTimes, uuids, etags)
	if err != nil {
		return fmt.Errorf("error while inserting object refs with psql: %w", err)
	}

	return nil
}

var deleteObjectQuery = `DELETE FROM object_refs WHERE path = ANY($1);`

func deleteObjectRefPSQL(ctx context.Context, tx pgx.Tx, paths []Path) error {
	ps := make([]string, 0, len(paths))
	for _, path := range paths {
		ps = append(ps, path.String())
	}

	_, err := tx.Exec(ctx, deleteObjectQuery, ps)
	if err != nil {
		return fmt.Errorf("error while deleting object refs in psql: %w", err)
	}

	return nil
}

// CreateCalendarObject implements Store.
func (o *Online) CreateCalendarObject(ctx context.Context, object Object) error {
	// First, check if we already have an object under this path. In that case,
	// we want to reuse the Ref as the S3's key.
	ref, exists := o.objectRefTable.Get(object.Path)
	if !exists {
		// Let's create a new object ref for this path
		ref = objectRef{
			Path:    object.Path,
			ModTime: object.ModTime,
			Ref:     uuid.New(),
			// No need to set ETag here, it will be updated after we store the object
			// in S3.
		}
	}

	etag, err := o.objectCache.Put(ctx, ref.s3Key(), object.Data)
	if err != nil {
		return fmt.Errorf("error while storing calendar object in s3: %w", err)
	}
	// Ensure we store the correct etag
	ref.ETag = etag

	_, err = runInTx(o.objectRefTable, func(ctx context.Context, tx *pgcache.TableTx[Path, objectRef]) (unit, error) {
		return unit{}, tx.Store(ctx, []objectRef{ref})
	})(ctx)

	return err
}

// ListCalendarObjects implements Store.
func (o *Online) ListCalendarObjects(ctx context.Context, basePath Path) ([]Object, error) {
	endPath := Path(fmt.Sprintf("%s%c", basePath, utf8.MaxRune))
	refs := o.objectRefTable.Between(basePath, endPath)

	var objects []Object

	for ref := range refs {
		object, err := o.getObjectFromRef(ctx, ref)
		if err != nil {
			return nil, fmt.Errorf("error while fetching object from ref at path %q: %w", ref.Path, err)
		}

		objects = append(objects, object)
	}

	return objects, nil
}

// GetCalendarObject implements Store.
func (o *Online) GetCalendarObject(ctx context.Context, path Path) (*Object, error) {
	ref, found := o.objectRefTable.Get(path)
	if !found {
		return nil, ErrCalendarObjectNotFound
	}

	object, err := o.getObjectFromRef(ctx, ref)
	if err != nil {
		return nil, err
	}

	return &object, nil
}

// DeleteCalendarObject implements Store.
func (o *Online) DeleteCalendarObject(ctx context.Context, path Path) error {
	ref, exists := o.objectRefTable.Get(path)
	if !exists {
		return fmt.Errorf("attempted to delete missing calendar object at %q: %w", path, ErrCalendarObjectNotFound)
	}

	_, err := runInTx(o.objectRefTable, func(ctx context.Context, tx *pgcache.TableTx[Path, objectRef]) (unit, error) {
		return unit{}, tx.Delete(ctx, []Path{path})
	})(ctx)
	if err != nil {
		return fmt.Errorf("error while removing object ref: %w", err)
	}

	if err := o.objectCache.Remove(ctx, ref.s3Key()); err != nil {
		return fmt.Errorf("error while removing entry from s3: %w", err)
	}

	return nil
}

func (o *Online) getObjectFromRef(ctx context.Context, ref objectRef) (Object, error) {
	data, etag, err := o.objectCache.Get(ctx, ref.s3Key(), ref.ETag)
	if err != nil {
		return Object{}, fmt.Errorf("error while fetching calendar object from S3: %w", err)
	}

	if etag != ref.ETag {
		o.logger.WarnContext(
			ctx,
			"mismatched etag after fresh query from S3",
			"key",
			ref.s3Key(),
			"expected",
			ref.ETag,
			"received",
			etag,
		)
	}

	object := Object{
		Path:    ref.Path,
		ModTime: ref.ModTime,
		Data:    data,
		// Use the ground truth from s3 for etag
		ETag: string(etag),
	}

	return object, nil
}
