package pgcache

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/teapotovh/teapot/lib/observability"
	"github.com/teapotovh/teapot/lib/run"
)

var (
	ErrNotLoaded        = errors.New("not loaded yet")
	ErrNotListening     = errors.New("not listening yet")
	ErrInvalidUpdate    = errors.New("invalid update")
	ErrAlreadyCommitted = errors.New("already committed")
)

const (
	BackoffInitialInterval = 100 * time.Millisecond
	BackoffMultiplier      = 1.5
	BackoffMaxRetriess     = 3
)

type (
	Table[K Key[K], T Object[K]] struct {
		logger *slog.Logger

		table string
		// pool is used to perform dataplane operations with a connection pool
		pool *pgxpool.Pool
		// nm encapsulates the notification logic
		nm *notificationManager[K]
		// cache stores the whole table view and gets updated upon invalidations.
		// the updates are eventually consistent, thus stale data may be read.
		cache  *SortedMap[K, T]
		loaded atomic.Bool

		listFunc   List[K, T]
		getFunc    Get[K, T]
		storeFunc  Store[K, T]
		deleteFunc Delete[K, T]
	}

	update[K Key[K], T Object[K]] struct {
		store  *T
		delete *K
	}

	TableTx[K Key[K], T Object[K]] struct {
		table     *Table[K, T]
		tx        pgx.Tx
		mu        sync.Mutex
		updates   []update[K, T]
		committed atomic.Bool
	}
)

func trackDurationList[K Key[K], T Object[K]](table string, list List[K, T]) List[K, T] {
	return func(ctx context.Context, conn *pgxpool.Pool) (objects []T, err error) {
		start := time.Now()

		defer func() {
			operationDuration.WithLabelValues(table, operationList, status(err)).Observe(time.Since(start).Seconds())
		}()

		return list(ctx, conn)
	}
}

func trackDurationGet[K Key[K], T Object[K]](table string, get Get[K, T]) Get[K, T] {
	return func(ctx context.Context, conn *pgxpool.Pool, keys []K) (objects []T, err error) {
		start := time.Now()

		defer func() {
			operationDuration.WithLabelValues(table, operationGet, status(err)).Observe(time.Since(start).Seconds())
		}()

		return get(ctx, conn, keys)
	}
}

func trackDurationStore[K Key[K], T Object[K]](table string, store Store[K, T]) Store[K, T] {
	return func(ctx context.Context, tx pgx.Tx, objects []T) (err error) {
		start := time.Now()

		defer func() {
			operationDuration.WithLabelValues(table, operationStore, status(err)).Observe(time.Since(start).Seconds())
		}()

		return store(ctx, tx, objects)
	}
}

func trackDurationDelete[K Key[K], T Object[K]](table string, del Delete[K, T]) Delete[K, T] {
	return func(ctx context.Context, tx pgx.Tx, keys []K) (err error) {
		start := time.Now()

		defer func() {
			operationDuration.WithLabelValues(table, operationDelete, status(err)).Observe(time.Since(start).Seconds())
		}()

		return del(ctx, tx, keys)
	}
}

func NewTable[K Key[K], T Object[K]](pool *pgxpool.Pool,
	table string,
	fromString func(str string) (*K, error),
	list List[K, T],
	get Get[K, T],
	store Store[K, T],
	del Delete[K, T],
	logger *slog.Logger) (*Table[K, T], error) {
	poolcfg := pool.Config()

	nm, err := newNotificationManager(poolcfg.ConnConfig, table, fromString)
	if err != nil {
		return nil, fmt.Errorf("error while building the notification manager: %w", err)
	}

	t := Table[K, T]{
		logger: logger,

		table: table,
		pool:  pool,
		nm:    nm,

		cache: NewSortedMap[K, T](),

		listFunc:   trackDurationList(table, list),
		getFunc:    trackDurationGet(table, get),
		storeFunc:  trackDurationStore(table, store),
		deleteFunc: trackDurationDelete(table, del),
	}

	return &t, nil
}

func (t *Table[K, T]) store(key K, object T) {
	if _, existed := t.cache.Load(key); !existed {
		defer objectsTotal.WithLabelValues(t.table).Inc()
	}

	t.cache.Store(key, object)
}

func (t *Table[K, T]) delete(key K) {
	if _, existed := t.cache.Load(key); existed {
		defer objectsTotal.WithLabelValues(t.table).Desc()
	}

	t.cache.Delete(key)
}

func (t *Table[K, T]) Get(key K) (T, bool) {
	defer loadsTotal.WithLabelValues(t.table, approachGet).Inc()
	return t.cache.Load(key)
}

func (t *Table[K, T]) List() iter.Seq[T] {
	return func(yield func(object T) bool) {
		for _, object := range t.cache.All() {
			loadsTotal.WithLabelValues(t.table, approachList).Inc()

			if !yield(object) {
				break
			}
		}
	}
}

func (t *Table[K, T]) From(key K) iter.Seq[T] {
	return func(yield func(object T) bool) {
		for _, object := range t.cache.From(key) {
			loadsTotal.WithLabelValues(t.table, approachFrom).Inc()

			if !yield(object) {
				break
			}
		}
	}
}

func (t *Table[K, T]) Between(from, to K) iter.Seq[T] {
	return func(yield func(object T) bool) {
		for _, object := range t.cache.Between(from, to) {
			loadsTotal.WithLabelValues(t.table, approachBetween).Inc()

			if !yield(object) {
				break
			}
		}
	}
}

func (t *Table[K, T]) Begin(ctx context.Context) (*TableTx[K, T], error) {
	tx, err := t.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("error while beginning transaction: %w", err)
	}

	ttx := TableTx[K, T]{
		table: t,
		tx:    tx,
	}

	return &ttx, nil
}

func (ttx *TableTx[K, T]) Store(ctx context.Context, objects []T) error {
	if ttx.committed.Load() {
		return ErrAlreadyCommitted
	}

	if err := ttx.table.storeFunc(ctx, ttx.tx, objects); err != nil {
		return fmt.Errorf("error while performing the store operation: %w", err)
	}

	ttx.mu.Lock()
	defer ttx.mu.Unlock()

	for _, object := range objects {
		ttx.updates = append(ttx.updates, update[K, T]{
			store: &object,
		})
	}

	return nil
}

func (ttx *TableTx[K, T]) Delete(ctx context.Context, keys []K) error {
	if ttx.committed.Load() {
		return ErrAlreadyCommitted
	}

	if err := ttx.table.deleteFunc(ctx, ttx.tx, keys); err != nil {
		return fmt.Errorf("error while performing the delete operation: %w", err)
	}

	ttx.mu.Lock()
	defer ttx.mu.Unlock()

	for _, key := range keys {
		ttx.updates = append(ttx.updates, update[K, T]{
			delete: &key,
		})
	}

	return nil
}

func (ttx *TableTx[K, T]) Commit(ctx context.Context) error {
	if ttx.committed.Load() {
		return ErrAlreadyCommitted
	}

	ttx.mu.Lock()
	defer ttx.mu.Unlock()

	events := make([]Event[K], 0, len(ttx.updates))
	for _, update := range ttx.updates {
		switch {
		case update.store != nil:
			events = append(events, Event[K]{Type: EventTypeStore, Key: (*update.store).Key()})
		case update.delete != nil:
			events = append(events, Event[K]{Type: EventTypeStore, Key: *update.delete})
		default:
			return ErrInvalidUpdate
		}
	}

	if err := ttx.table.nm.Notify(ctx, events); err != nil {
		return fmt.Errorf("error while sending notification during commit: %w", err)
	}

	if err := ttx.tx.Commit(ctx); err != nil {
		return fmt.Errorf("error while committing: %w", err)
	}

	for _, update := range ttx.updates {
		switch {
		case update.store != nil:
			object := *update.store
			ttx.table.store(object.Key(), object)
		case update.delete != nil:
			key := *update.delete
			ttx.table.delete(key)
		default:
			return ErrInvalidUpdate
		}
	}

	ttx.updates = ttx.updates[:0]
	ttx.committed.Store(true)

	return nil
}

func (t *Table[K, T]) handleEvents(ctx context.Context, events []Event[K]) (err error) {
	start := time.Now()

	defer func() {
		if err == nil {
			notificationUpdateDuration.WithLabelValues(t.table).Observe(time.Since(start).Seconds())
		}
	}()

	var refresh []K

	for _, event := range events {
		t.logger.Info("updating cache due to notification event", "event", event.String())

		switch event.Type {
		case EventTypeStore:
			refresh = append(refresh, event.Key)

		case EventTypeDelete:
			t.delete(event.Key)

		default:
			return ErrUnexpectedEvenType
		}
	}

	if len(refresh) > 0 {
		f := func() (int, error) {
			objects, err := t.getFunc(ctx, t.pool, refresh)
			if err != nil {
				return 0, fmt.Errorf("error while fetching %d objects to refresh: %w", len(refresh), err)
			}

			for _, object := range objects {
				t.store(object.Key(), object)
			}

			return len(objects), nil
		}

		expoBackoff := backoff.NewExponentialBackOff()
		expoBackoff.InitialInterval = BackoffInitialInterval
		expoBackoff.Multiplier = BackoffMultiplier

		n, err := backoff.Retry(ctx, f, backoff.WithMaxTries(BackoffMaxRetriess), backoff.WithBackOff(expoBackoff))
		if err != nil {
			return fmt.Errorf("error while handling notification: %w", err)
		}

		if n != len(refresh) {
			// We culd run into this if we have concurrent update/delete operations
			// coming from different database connections, meaning that an object
			// that was just updated is immediately deleted by another connection.
			//
			// This is fine, as the cache will eventually receive the delete event
			// and become consistent again. Until then the data is retained in the
			// cache for correctness.
			t.logger.WarnContext(
				ctx,
				"received an unexpected amount of objects when refreshing the cache",
				"expected",
				len(refresh),
				"retrieved",
				n,
			)
		}
	}

	return nil
}

// Run implements run.Runnable.
func (t *Table[K, T]) Run(ctx context.Context, notify run.Notify) error {
	objects, err := t.listFunc(ctx, t.pool)
	if err != nil {
		return fmt.Errorf("error while fetching all objects to populate the cache: %w", err)
	}

	for _, object := range objects {
		t.store(object.Key(), object)
	}

	t.logger.Info("populated initial object cache", "table", t.table, "count", len(objects))

	err = t.nm.Listen(ctx)
	if err != nil {
		return fmt.Errorf("error while setting up notifications listener: %w", err)
	}

	notify.Notify()
	t.loaded.Store(true)

	for {
		events, err := t.nm.Next(ctx)
		if err != nil {
			// If the context was canceled, we can ignore the error we get from the
			// postgres connection - it's due to the context cancelation.
			if ctx.Err() != nil {
				return nil //nolint:nilerr
			}

			return fmt.Errorf("error while getting the next notification: %w", err)
		}

		t.logger.Info("received notifications", "count", len(events))

		if err := t.handleEvents(ctx, events); err != nil {
			return fmt.Errorf("error while performing cache update due to notification: %w", err)
		}
	}
}

// Metrics implements observability.Metrics.
func (t *Table[K, T]) Metrics() []prometheus.Collector {
	return []prometheus.Collector{
		objectsTotal,
		loadsTotal,
		notificationUpdateDuration,
		operationDuration,
		transactionDuration,
	}
}

func (t *Table[K, T]) ping(ctx context.Context) error {
	if err := t.pool.Ping(ctx); err != nil {
		return fmt.Errorf("error while pinging pool: %w", err)
	}

	return t.nm.Ping(ctx)
}

func (t *Table[K, T]) isLoaded(_ context.Context) error {
	if t.loaded.Load() {
		return nil
	}

	return ErrNotLoaded
}

func (t *Table[K, T]) isListening(_ context.Context) error {
	if t.nm.IsListening() {
		return nil
	}

	return ErrNotListening
}

// ReadinessChecks implements observability.ReadinessChecks.
func (t *Table[K, T]) ReadinessChecks() map[string]observability.Check {
	return map[string]observability.Check{
		"pgcache/" + t.table + "/ping":      observability.CheckFunc(t.ping),
		"pgcache/" + t.table + "/loaded":    observability.CheckFunc(t.isLoaded),
		"pgcache/" + t.table + "/listening": observability.CheckFunc(t.isListening),
	}
}

// Ensure *Table implements run.Runnable.
var _ run.Runnable = &Table[StringKey, Object[StringKey]]{}

// Ensure *Table implements observability.Metrics.
var _ observability.Metrics = &Table[StringKey, Object[StringKey]]{}

// Ensure *Table implements observability.ReadinessChecks.
var _ observability.ReadinessChecks = &Table[StringKey, Object[StringKey]]{}
