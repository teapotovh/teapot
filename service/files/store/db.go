package store

import (
	"context"
)

// DB is the metadata database used by the store.
type DB interface {
	// Begin starts a read-write transaction.
	Begin(ctx context.Context) (Tx, error)
	// BeginSnapshot starts a repeatable read transaction.
	BeginSnapshot(ctx context.Context) (Tx, error)
	// Ping checks that the database is reachable.
	Ping(ctx context.Context) error
	// Close closes the database connections.
	Close()
}

// Tx is one metadata transaction.
type Tx interface {
	// BumpRevision increments the global revision and returns the new value.
	BumpRevision(ctx context.Context) (uint64, error)
	// CurrentRevision returns the global revision without changing it.
	CurrentRevision(ctx context.Context) (uint64, error)
	// EnsureRoot creates the root inode when it is missing.
	EnsureRoot(ctx context.Context, uid int64, revision uint64) error
	// NextNodeID returns a new node id for the user.
	NextNodeID(ctx context.Context, uid int64) (uint64, error)
	// Inode returns the live inode with the given id.
	Inode(ctx context.Context, uid int64, nodeID uint64) (Inode, bool, error)
	// Child returns the live child with the given name.
	Child(ctx context.Context, uid int64, parentID uint64, name string) (Inode, bool, error)
	// Children returns all live children of a directory, ordered by name.
	Children(ctx context.Context, uid int64, parentID uint64) ([]Inode, error)
	// Upsert stores an inode.
	Upsert(ctx context.Context, inode Inode) error
	// ChangedSince returns all inodes changed after a revision, ordered by revision.
	ChangedSince(ctx context.Context, revision uint64) ([]Inode, error)
	// SnapshotPage returns one page of inodes ordered by parent id and name.
	SnapshotPage(
		ctx context.Context,
		afterRevision uint64,
		cursor Cursor,
		limit int,
	) (inodes []Inode, next Cursor, more bool, err error)
	// OldestTombstone returns the oldest retained tombstone revision.
	OldestTombstone(ctx context.Context) (uint64, bool, error)
	// Notify sends a notification at commit time.
	Notify(ctx context.Context, payload []byte) error
	// Commit commits the transaction.
	Commit(ctx context.Context) error
	// Rollback rolls back the transaction.
	Rollback(ctx context.Context) error
}
