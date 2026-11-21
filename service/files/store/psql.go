package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/teapotovh/teapot/lib/files"
)

const inodeColumns = `
	uid, node_id, parent_id, name, kind, size, mtime, hash, s3_key, deleted, revision
`

// ErrInvalidKind reports an unknown inode kind.
var ErrInvalidKind = errors.New("invalid inode kind")

var (
	revisionQuery = `
		UPDATE revision SET value = value + 1 WHERE id = 1 RETURNING value;
	`

	currentRevisionQuery = `
		SELECT value FROM revision WHERE id = 1;
	`

	ensureRootQuery = `
		INSERT INTO inodes (uid, node_id, parent_id, name, kind, size, mtime, hash, s3_key, deleted, revision)
		VALUES ($1, 1, 1, '', 0, 0, now(), '', '', false, $2)
		ON CONFLICT (uid, node_id) DO NOTHING;
	`

	nextNodeIDQuery = `
		SELECT COALESCE(MAX(node_id), 0) + 1 FROM inodes WHERE uid = $1;
	`

	inodeQuery = `
		SELECT ` + inodeColumns + `
		FROM inodes
		WHERE uid = $1 AND node_id = $2 AND deleted = false;
	`

	childQuery = `
		SELECT ` + inodeColumns + `
		FROM inodes
		WHERE uid = $1 AND parent_id = $2 AND name = $3 AND deleted = false
		ORDER BY node_id DESC
		LIMIT 1;
	`

	childrenQuery = `
		SELECT ` + inodeColumns + `
		FROM inodes
		WHERE uid = $1 AND parent_id = $2 AND deleted = false
		ORDER BY name;
	`

	upsertQuery = `
		INSERT INTO inodes (uid, node_id, parent_id, name, kind, size, mtime, hash, s3_key, deleted, revision)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (uid, node_id) DO UPDATE SET
			parent_id = EXCLUDED.parent_id,
			name = EXCLUDED.name,
			kind = EXCLUDED.kind,
			size = EXCLUDED.size,
			mtime = EXCLUDED.mtime,
			hash = EXCLUDED.hash,
			s3_key = EXCLUDED.s3_key,
			deleted = EXCLUDED.deleted,
			revision = EXCLUDED.revision;
	`

	changedSinceQuery = `
		SELECT ` + inodeColumns + `
		FROM inodes
		WHERE revision > $1
		ORDER BY revision;
	`

	snapshotPageQuery = `
		SELECT ` + inodeColumns + `
		FROM inodes
		WHERE revision > $1
			AND (deleted = false OR $1 > 0)
			AND (parent_id, name) > ($2, $3)
		ORDER BY parent_id, name
		LIMIT $4;
	`

	oldestTombstoneQuery = `
		SELECT MIN(revision) FROM inodes WHERE deleted = true;
	`
)

// PSQL is the PostgreSQL metadata backend.
type PSQL struct {
	pool   *pgxpool.Pool
	notify Notifications
}

// Begin implements DB.
func (p *PSQL) Begin(ctx context.Context) (Tx, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("error while starting transaction: %w", err)
	}

	return &psqlTx{tx: tx, notify: p.notify}, nil
}

// BeginSnapshot implements DB.
func (p *PSQL) BeginSnapshot(ctx context.Context) (Tx, error) {
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return nil, fmt.Errorf("error while starting snapshot transaction: %w", err)
	}

	return &psqlTx{tx: tx, notify: p.notify}, nil
}

// Ping implements DB.
func (p *PSQL) Ping(ctx context.Context) error {
	return p.pool.Ping(ctx)
}

// Close implements DB.
func (p *PSQL) Close() {
	p.pool.Close()
}

type psqlTx struct {
	tx     pgx.Tx
	notify Notifications
}

// BumpRevision implements Tx.
func (t *psqlTx) BumpRevision(ctx context.Context) (uint64, error) {
	var revision uint64
	if err := t.tx.QueryRow(ctx, revisionQuery).Scan(&revision); err != nil {
		return 0, fmt.Errorf("error while bumping the revision: %w", err)
	}

	return revision, nil
}

// CurrentRevision implements Tx.
func (t *psqlTx) CurrentRevision(ctx context.Context) (uint64, error) {
	var revision uint64
	if err := t.tx.QueryRow(ctx, currentRevisionQuery).Scan(&revision); err != nil {
		return 0, fmt.Errorf("error while reading the revision: %w", err)
	}

	return revision, nil
}

// EnsureRoot implements Tx.
func (t *psqlTx) EnsureRoot(ctx context.Context, uid int64, revision uint64) error {
	_, err := t.tx.Exec(ctx, ensureRootQuery, uid, revision)
	if err != nil {
		return fmt.Errorf("error while ensuring the root inode: %w", err)
	}

	return nil
}

// NextNodeID implements Tx.
func (t *psqlTx) NextNodeID(ctx context.Context, uid int64) (uint64, error) {
	var nodeID uint64
	if err := t.tx.QueryRow(ctx, nextNodeIDQuery, uid).Scan(&nodeID); err != nil {
		return 0, fmt.Errorf("error while allocating a node id: %w", err)
	}

	return nodeID, nil
}

// Inode implements Tx.
func (t *psqlTx) Inode(ctx context.Context, uid int64, nodeID uint64) (Inode, bool, error) {
	inode, err := scanInode(t.tx.QueryRow(ctx, inodeQuery, uid, int64(nodeID))) //nolint:gosec
	if errors.Is(err, pgx.ErrNoRows) {
		return Inode{}, false, nil
	}

	if err != nil {
		return Inode{}, false, fmt.Errorf("error while reading the inode: %w", err)
	}

	return inode, true, nil
}

// Child implements Tx.
func (t *psqlTx) Child(ctx context.Context, uid int64, parentID uint64, name string) (Inode, bool, error) {
	inode, err := scanInode(t.tx.QueryRow(ctx, childQuery, uid, int64(parentID), name)) //nolint:gosec
	if errors.Is(err, pgx.ErrNoRows) {
		return Inode{}, false, nil
	}

	if err != nil {
		return Inode{}, false, fmt.Errorf("error while reading the child inode: %w", err)
	}

	return inode, true, nil
}

// Children implements Tx.
func (t *psqlTx) Children(ctx context.Context, uid int64, parentID uint64) ([]Inode, error) {
	rows, err := t.tx.Query(ctx, childrenQuery, uid, int64(parentID)) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("error while listing children: %w", err)
	}

	return scanInodes(rows)
}

// Upsert implements Tx.
func (t *psqlTx) Upsert(ctx context.Context, inode Inode) error {
	kind, err := inodeKind(inode.Kind)
	if err != nil {
		return err
	}

	hash := string(inode.Hash)

	_, err = t.tx.Exec(
		ctx,
		upsertQuery,
		inode.UID,
		int64(inode.NodeID),   //nolint:gosec
		int64(inode.ParentID), //nolint:gosec
		inode.Name,
		kind,
		inode.Size,
		inode.ModTime,
		hash,
		inode.S3Key,
		inode.Deleted,
		int64(inode.Revision), //nolint:gosec
	)
	if err != nil {
		return fmt.Errorf("error while storing the inode: %w", err)
	}

	return nil
}

// ChangedSince implements Tx.
func (t *psqlTx) ChangedSince(ctx context.Context, revision uint64) ([]Inode, error) {
	rows, err := t.tx.Query(ctx, changedSinceQuery, int64(revision)) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("error while listing changed inodes: %w", err)
	}

	return scanInodes(rows)
}

// SnapshotPage implements Tx.
func (t *psqlTx) SnapshotPage(
	ctx context.Context,
	afterRevision uint64,
	cursor Cursor,
	limit int,
) ([]Inode, Cursor, bool, error) {
	rows, err := t.tx.Query(
		ctx,
		snapshotPageQuery,
		int64(afterRevision),   //nolint:gosec
		int64(cursor.ParentID), //nolint:gosec
		cursor.Name,
		limit,
	)
	if err != nil {
		return nil, Cursor{}, false, fmt.Errorf("error while reading the snapshot page: %w", err)
	}

	inodes, err := scanInodes(rows)
	if err != nil {
		return nil, Cursor{}, false, err
	}

	next := cursor

	if len(inodes) > 0 {
		last := inodes[len(inodes)-1]
		next = Cursor{ParentID: last.ParentID, Name: last.Name}
	}

	return inodes, next, len(inodes) == limit, nil
}

// OldestTombstone implements Tx.
func (t *psqlTx) OldestTombstone(ctx context.Context) (uint64, bool, error) {
	var revision *int64
	if err := t.tx.QueryRow(ctx, oldestTombstoneQuery).Scan(&revision); err != nil {
		return 0, false, fmt.Errorf("error while reading the oldest tombstone: %w", err)
	}

	if revision == nil {
		return 0, false, nil
	}

	return uint64(*revision), true, nil //nolint:gosec // Revisions are non-negative.
}

// Notify implements Tx.
func (t *psqlTx) Notify(ctx context.Context, payload []byte) error {
	return t.notify.Notify(ctx, t.tx, defaultChannel, payload)
}

// Commit implements Tx.
func (t *psqlTx) Commit(ctx context.Context) error {
	return t.tx.Commit(ctx)
}

// Rollback implements Tx.
func (t *psqlTx) Rollback(ctx context.Context) error {
	return t.tx.Rollback(ctx)
}

type scanner interface {
	Scan(dest ...any) error
}

func scanInode(row scanner) (Inode, error) {
	var (
		inode    Inode
		nodeID   int64
		parentID int64
		kind     int16
		hash     string
		revision int64
	)

	err := row.Scan(
		&inode.UID,
		&nodeID,
		&parentID,
		&inode.Name,
		&kind,
		&inode.Size,
		&inode.ModTime,
		&hash,
		&inode.S3Key,
		&inode.Deleted,
		&revision,
	)
	if err != nil {
		return Inode{}, err
	}

	switch kind {
	case dirKind:
		inode.Kind = files.KindDirectory
	case fileKind:
		inode.Kind = files.KindFile
	default:
		return Inode{}, ErrInvalidKind
	}

	inode.NodeID = uint64(nodeID)     //nolint:gosec
	inode.ParentID = uint64(parentID) //nolint:gosec
	inode.Hash = files.Hash(hash)
	inode.Revision = uint64(revision) //nolint:gosec

	return inode, nil
}

func scanInodes(rows pgx.Rows) ([]Inode, error) {
	defer rows.Close()

	inodes := make([]Inode, 0)

	for rows.Next() {
		inode, err := scanInode(rows)
		if err != nil {
			return nil, fmt.Errorf("error while scanning inode: %w", err)
		}

		inodes = append(inodes, inode)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error while reading inodes: %w", err)
	}

	return inodes, nil
}

// Ensure *PSQL implements DB.
var _ DB = (*PSQL)(nil)
