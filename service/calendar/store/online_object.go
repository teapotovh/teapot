package store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/sync/errgroup"

	"github.com/teapotovh/teapot/lib/s3cache"
)

const MaxRequestsInParallel = 8

// ObjectRef is a calendar object reference stored in psql.
type ObjectRef struct {
	Path    Path
	ModTime time.Time

	Ref  uuid.UUID
	ETag s3cache.Hash
}

func (ref ObjectRef) s3Key() string {
	return ref.Ref.String()
}

func parseObjectRows(rows pgx.Rows) (refs []ObjectRef, err error) {
	defer rows.Close()

	for rows.Next() {
		var (
			ref   ObjectRef
			refID pgtype.UUID
		)

		if err := rows.Scan(&ref.Path, &ref.ModTime, &refID, &ref.ETag); err != nil {
			return nil, fmt.Errorf("could not extract four columns from psql list: %w", err)
		}

		ref.Ref = uuid.UUID(refID.Bytes)

		refs = append(refs, ref)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("could not read all psql results: %w", err)
	}

	return refs, nil
}

var (
	listObjectQuery = `
		SELECT path, mod_time, ref, etag
		FROM object_refs
		ORDER BY path;
	`

	getObjectQuery = `
		SELECT path, mod_time, ref, etag
		FROM object_refs
		WHERE path = $1
		LIMIT 1;
	`

	rangeObjectQuery = `
		SELECT path, mod_time, ref, etag
		FROM object_refs
		WHERE path >= $1
			AND path < $2
		ORDER BY path;
	`

	storeObjectQuery = `
		INSERT INTO object_refs (path, mod_time, ref, etag)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (path) DO UPDATE
		SET mod_time = EXCLUDED.mod_time, etag = EXCLUDED.etag;
	`

	deleteObjectQuery = `DELETE FROM object_refs WHERE path = $1;`
)

// PutCalendarObject implements Store.
func (o *Online) PutCalendarObject(ctx context.Context, object Object) (bool, error) {
	// First, check if we already have an object under this path. In that case,
	// we want to reuse the Ref as the S3's key.
	ref, err := o.getObjectRef(ctx, object.Path)
	created := errors.Is(err, ErrCalendarObjectNotFound)

	if err != nil && !created {
		return false, err
	}

	if created {
		// Let's create a new object ref for this path
		ref = ObjectRef{
			Path:    object.Path,
			ModTime: object.ModTime,
			Ref:     uuid.New(),
			// No need to set ETag here, it will be updated after we store the object
			// in S3.
		}
	}

	ref.ModTime = object.ModTime

	etag, err := o.objectCache.Put(ctx, ref.s3Key(), bytes.NewReader(object.Data))
	if err != nil {
		return false, fmt.Errorf("error while storing calendar object in s3: %w", err)
	}
	// Ensure we store the correct etag
	ref.ETag = etag

	err = runInTx(ctx, o, func(ctx context.Context, tx Transaction) error {
		return tx.StoreObjectRef(ctx, ref)
	})
	if err != nil {
		return false, err
	}

	return created, nil
}

// ListCalendarObjects implements Store.
func (o *Online) ListCalendarObjects(ctx context.Context, basePath Path) (objects []Object, err error) {
	operation := metricsOperationRange
	if basePath == "" {
		operation = metricsOperationList
	}

	start := time.Now()
	defer func() { o.metrics.observeOperation(operation, start, err) }()

	var refs []ObjectRef

	if basePath == "" {
		rows, err := o.pool.Query(ctx, listObjectQuery)
		if err != nil {
			return nil, fmt.Errorf("error while listing object refs from psql: %w", err)
		}

		refs, err = parseObjectRows(rows)
		if err != nil {
			return nil, err
		}
	} else {
		rows, err := o.pool.Query(ctx, rangeObjectQuery, basePath.String(), prefixEnd(basePath).String())
		if err != nil {
			return nil, fmt.Errorf("error while listing object refs from psql: %w", err)
		}

		refs, err = parseObjectRows(rows)
		if err != nil {
			return nil, err
		}
	}

	objects = make([]Object, len(refs))
	eg, ctx := errgroup.WithContext(ctx)
	eg.SetLimit(MaxRequestsInParallel)

	for i, ref := range refs {
		eg.Go(func() error {
			object, err := o.getObjectFromRef(ctx, ref)
			if err != nil {
				return fmt.Errorf("error while fetching object #%d from ref at path %q: %w", i, ref.Path, err)
			}

			objects[i] = object

			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}

	return objects, nil
}

// GetCalendarObject implements Store.
func (o *Online) GetCalendarObject(ctx context.Context, path Path) (object *Object, err error) {
	start := time.Now()
	defer func() { o.metrics.observeOperation(metricsOperationGet, start, err) }()

	ref, err := o.getObjectRef(ctx, path)
	if err != nil {
		return nil, err
	}

	got, err := o.getObjectFromRef(ctx, ref)
	if err != nil {
		return nil, err
	}

	return &got, nil
}

func (o *Online) getObjectRef(ctx context.Context, path Path) (ObjectRef, error) {
	rows, err := o.pool.Query(ctx, getObjectQuery, path.String())
	if err != nil {
		return ObjectRef{}, fmt.Errorf("error while getting object ref from psql: %w", err)
	}

	refs, err := parseObjectRows(rows)
	if err != nil {
		return ObjectRef{}, err
	}

	if len(refs) == 0 {
		return ObjectRef{}, ErrCalendarObjectNotFound
	}

	return refs[0], nil
}

// DeleteCalendarObject implements Store.
func (o *Online) DeleteCalendarObject(ctx context.Context, path Path) error {
	ref, err := o.getObjectRef(ctx, path)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return fmt.Errorf("attempted to delete missing calendar object at %q: %w", path, ErrCalendarObjectNotFound)
		}

		return err
	}

	err = runInTx(ctx, o, func(ctx context.Context, tx Transaction) error {
		return tx.DeleteObjectRef(ctx, path)
	})
	if err != nil {
		return fmt.Errorf("error while removing object ref: %w", err)
	}

	if err := o.objectCache.Remove(ctx, ref.s3Key()); err != nil {
		return fmt.Errorf("error while removing entry from s3: %w", err)
	}

	return nil
}

// StoreObjectRef implements Transaction.
func (t *psqlTransaction) StoreObjectRef(ctx context.Context, ref ObjectRef) (err error) {
	start := time.Now()
	defer func() { t.metrics.observeOperation(metricsOperationStore, start, err) }()

	_, err = t.tx.Exec(
		ctx,
		storeObjectQuery,
		ref.Path.String(),
		ref.ModTime,
		pgtype.UUID{Bytes: ref.Ref, Valid: true},
		ref.ETag.String(),
	)
	if err != nil {
		return fmt.Errorf("error while inserting object ref with psql: %w", err)
	}

	return nil
}

// DeleteObjectRef implements Transaction.
func (t *psqlTransaction) DeleteObjectRef(ctx context.Context, path Path) (err error) {
	start := time.Now()
	defer func() { t.metrics.observeOperation(metricsOperationDelete, start, err) }()

	if _, err := t.tx.Exec(ctx, deleteObjectQuery, path.String()); err != nil {
		return fmt.Errorf("error while deleting object ref in psql: %w", err)
	}

	return nil
}

func (o *Online) getObjectFromRef(ctx context.Context, ref ObjectRef) (Object, error) {
	reader, etag, err := o.objectCache.Get(ctx, ref.s3Key(), ref.ETag)
	if err != nil {
		return Object{}, fmt.Errorf("error while fetching calendar object from S3: %w", err)
	}

	data, readErr := io.ReadAll(reader)

	if err := errors.Join(readErr, reader.Close()); err != nil {
		return Object{}, fmt.Errorf("error while reading calendar object from S3: %w", err)
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
