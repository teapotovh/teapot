package store

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"

	"git.sr.ht/~bitfehler/brant"
	"git.sr.ht/~bitfehler/brant/database/dialect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/teapotovh/teapot/lib/broker"
	"github.com/teapotovh/teapot/lib/pagedbtree"
	"github.com/teapotovh/teapot/lib/pgnotify"
	"github.com/teapotovh/teapot/lib/run"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Config is the configuration for the store.
type Config struct {
	// URL is the PostgreSQL connection string.
	URL string
	// Cache is the inode cache configuration.
	Cache CacheConfig
	// S3 is the blob store configuration.
	S3 S3Config
}

// Blob stores and reads file bytes.
type Blob interface {
	// Put stores the bytes read from r under key and returns their hash.
	Put(ctx context.Context, key string, r io.Reader) (Hash, error)
	// Get opens the blob stored under key.
	Get(ctx context.Context, key string) (io.ReadSeekCloser, error)
}

// ErrRevisionConflict reports that another write advanced the revision.
var ErrRevisionConflict = errors.New("revision conflict")

// DB is the metadata database used by the store.
type DB interface {
	// Load sends all live inodes and the current revision to fn.
	Load(ctx context.Context, fn func(revision uint64, inodes []Inode) error) error
	// Begin starts a write transaction at expectedRevision.
	Begin(ctx context.Context, expectedRevision uint64) (Tx, error)
	// Close closes the database connections.
	Close()
}

// Tx is one metadata write transaction.
type Tx interface {
	// Revision returns the revision of this transaction.
	Revision() uint64
	// Put stores an inode and returns its node id.
	Put(ctx context.Context, inode Inode) (uint64, error)
	// Delete removes an inode.
	Delete(ctx context.Context, uid int64, nodeID uint64) error
	// Commit commits the transaction and sends the notification payload.
	Commit(ctx context.Context, payload []byte) error
	// Rollback rolls back the transaction.
	Rollback(ctx context.Context) error
}

func rollbackOnError(ctx context.Context, tx Tx, err *error) {
	if *err == nil {
		return
	}

	*err = errors.Join(*err, tx.Rollback(ctx))
}

// Store is the authoritative file store.
type Store struct {
	logger *slog.Logger

	db            DB
	blob          Blob
	notifications pgnotify.Notifications

	cache  *cache
	events *broker.Broker[Event]

	revision atomic.Uint64
	synced   atomic.Bool
	applyMu  sync.Mutex

	metrics metrics
}

// NewStore creates a store from its dependencies.
func NewStore(
	config Config,
	db DB,
	blob Blob,
	notifications pgnotify.Notifications,
	logger *slog.Logger,
) (*Store, error) {
	return newStore(config, db, blob, notifications, osFS{}, logger)
}

func newStore(
	config Config,
	db DB,
	blob Blob,
	notifications pgnotify.Notifications,
	fs pagedbtree.FS,
	logger *slog.Logger,
) (*Store, error) {
	if config.Cache.Path == "" {
		return nil, fmt.Errorf("%w: cache path is required", pagedbtree.ErrInvalidConfig)
	}

	cache, err := openCache(fs, pagedbtree.Config{
		Path:   config.Cache.Path,
		Memory: config.Cache.Memory,
		Disk:   config.Cache.Disk,
	})
	if err != nil {
		return nil, err
	}

	store := &Store{
		logger: logger,

		db:            db,
		blob:          blob,
		notifications: notifications,

		cache:  cache,
		events: broker.NewBroker[Event](),
	}

	store.metrics.initMetrics()

	return store, nil
}

// NewPSQL creates a store backed by PostgreSQL and S3.
func NewPSQL(ctx context.Context, config Config, logger *slog.Logger) (*Store, error) {
	options := brant.DefaultOptions().
		WithTableName("_version").
		WithFilesystem(migrations).
		WithDataSourceName(config.URL)

	provider, err := brant.NewProvider(logger, dialect.Postgres, options)
	if err != nil {
		return nil, fmt.Errorf("error while constructing the migration provider: %w", err)
	}

	applied, err := provider.Up(ctx)
	if err != nil {
		return nil, fmt.Errorf("error while applying migrations: %w", err)
	}

	for _, migration := range applied {
		logger.Info("applied migration", "migration", migration)
	}

	if err := provider.Close(); err != nil {
		return nil, fmt.Errorf("error while closing the migration connection: %w", err)
	}

	poolConfig, err := pgxpool.ParseConfig(config.URL)
	if err != nil {
		return nil, fmt.Errorf("error while parsing the PostgreSQL URL: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return nil, fmt.Errorf("error while opening the PostgreSQL pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("error while connecting to PostgreSQL: %w", err)
	}

	connConfig, err := pgx.ParseConfig(config.URL)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("error while parsing the notification URL: %w", err)
	}

	notifications, err := pgnotify.NewManager(connConfig, logger.With("component", "notifications"))
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("error while creating the notification manager: %w", err)
	}

	blob, err := NewS3Blob(config.S3)
	if err != nil {
		err = errors.Join(err, notifications.Close())
		pool.Close()

		return nil, err
	}

	db := &PSQL{pool: pool, notify: notifications}

	store, err := NewStore(config, db, blob, notifications, logger)
	if err != nil {
		err = errors.Join(err, notifications.Close())
		pool.Close()

		return nil, err
	}

	return store, nil
}

// For returns the file system view for one user.
func (s *Store) For(uid int64) *FS {
	return &FS{store: s, uid: uid}
}

// Revision returns the last applied revision.
func (s *Store) Revision() uint64 {
	return s.revision.Load()
}

// Run implements run.Runnable.
func (s *Store) Run(ctx context.Context, notify run.Notify) (err error) {
	defer func() {
		err = errors.Join(err, s.notifications.Close())
		s.db.Close()
	}()

	ready := make(chan struct{})

	return run.Combine(
		&cacheRunner{cache: s.cache, ready: ready},
		s.events,
		&syncRunner{store: s, ready: ready},
	).Run(ctx, notify)
}

// cacheRunner starts both cache trees and signals when they are ready.
type cacheRunner struct {
	cache *cache
	ready chan<- struct{}
}

// Run implements run.Runnable.
func (r *cacheRunner) Run(ctx context.Context, notify run.Notify) error {
	return run.Combine(r.cache.nodes, r.cache.children).Run(ctx, &startupNotify{
		next:  notify,
		ready: r.ready,
	})
}

// syncRunner waits for the cache, loads the snapshot, then applies notifications.
type syncRunner struct {
	store *Store
	ready <-chan struct{}
}

// Run implements run.Runnable.
func (r *syncRunner) Run(ctx context.Context, notify run.Notify) error {
	select {
	case <-r.ready:
	case <-ctx.Done():
		return nil
	}

	if err := r.store.notifications.Listen(ctx, defaultChannel); err != nil {
		return fmt.Errorf("error while listening for notifications: %w", err)
	}

	if err := r.store.bootstrap(ctx); err != nil {
		return err
	}

	r.store.synced.Store(true)
	defer r.store.synced.Store(false)

	notify.Notify()

	return r.store.listen(ctx)
}

func (s *Store) setRevision(revision uint64) {
	s.revision.Store(revision)
	s.metrics.setRevision(revision)
}

func (s *Store) bootstrap(ctx context.Context) error {
	if err := s.cache.clear(); err != nil {
		return fmt.Errorf("error while clearing the cache: %w", err)
	}

	if err := s.db.Load(ctx, func(revision uint64, inodes []Inode) error {
		for _, inode := range inodes {
			if err := s.cache.apply(inode); err != nil {
				return err
			}
		}

		s.setRevision(revision)

		return nil
	}); err != nil {
		return fmt.Errorf("error while loading the snapshot: %w", err)
	}

	return nil
}

func (s *Store) listen(ctx context.Context) error {
	for {
		_, payload, err := s.notifications.Next(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil //nolint:nilerr // the context ended.
			}

			return fmt.Errorf("error while waiting for notifications: %w", err)
		}

		event, err := decodeEvent(payload)
		if err != nil {
			s.logger.ErrorContext(ctx, "could not decode notification", "err", err)
			continue
		}

		events, err := s.sync(ctx, event)
		if err != nil {
			s.logger.ErrorContext(ctx, "could not apply notification", "err", err)
			continue
		}

		s.publish(events)
	}
}

func (s *Store) sync(ctx context.Context, incoming Event) ([]Event, error) {
	s.applyMu.Lock()
	defer s.applyMu.Unlock()

	last := s.revision.Load()
	if incoming.Inode.Revision <= last {
		return nil, nil
	}

	if incoming.Inode.Revision > last+1 {
		s.logger.WarnContext(ctx, "notification gap, rebuilding cache", "last", last, "event", incoming.Inode.Revision)

		if err := s.bootstrap(ctx); err != nil {
			return nil, err
		}

		return nil, nil
	}

	if err := s.cache.apply(incoming.Inode); err != nil {
		return nil, err
	}

	s.setRevision(incoming.Inode.Revision)

	return []Event{incoming}, nil
}

func (s *Store) apply(ctx context.Context, event Event) error {
	events, err := s.sync(ctx, event)
	if err != nil {
		return err
	}

	s.publish(events)

	return nil
}

func (s *Store) publish(events []Event) {
	for _, event := range events {
		s.events.Publish(event)
	}
}

type startupNotify struct {
	next  run.Notify
	ready chan<- struct{}
	once  sync.Once
}

// Notify implements run.Notify.
func (n *startupNotify) Notify() {
	n.next.Notify()
	n.once.Do(func() { close(n.ready) })
}

type osFS struct{}

func (osFS) OpenFile(name string, flag int, perm os.FileMode) (pagedbtree.File, error) {
	return os.OpenFile(name, flag, perm) //nolint:gosec // The path comes from the configuration.
}

// Ensure the store implements the run interface.
var _ run.Runnable = (*Store)(nil)
