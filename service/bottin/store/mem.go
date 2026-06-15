package store

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/google/btree"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/teapotovh/teapot/lib/observability"
	"github.com/teapotovh/teapot/lib/run"
)

var ErrCommitted = errors.New("transaction already committed")

type mementry struct {
	entry  Entry
	prefix Prefix
}

func newmementry(entry Entry) mementry {
	return mementry{
		prefix: entry.DN.Prefix(),
		entry:  entry,
	}
}

func mementryFromPrefix(prefix Prefix) mementry {
	return mementry{
		prefix: prefix,
		entry: Entry{
			DN:         prefix.DN(),
			Attributes: Attributes{},
		},
	}
}

func mementryLess(a, b mementry) bool {
	return strings.Compare(a.prefix.String(), b.prefix.String()) == -1
}

type Mem struct {
	tr      *btree.BTreeG[mementry]
	mu      sync.RWMutex
	metrics metrics
}

func NewMem() *Mem {
	m := Mem{tr: btree.NewG(2, mementryLess)}
	m.metrics.initMetrics("mem")

	return &m
}

// Ping implements Store.
func (m *Mem) Ping(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	return nil
}

// List implements Store.
func (m *Mem) List(ctx context.Context, prefix Prefix, exact bool) (entries []Entry, err error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	start := mementryFromPrefix(prefix)
	end := mementryFromPrefix(prefixEnd(prefix))

	m.tr.AscendRange(start, end, func(entry mementry) bool {
		// For non-exact matches, continue looping and collect all results
		if !exact {
			entries = append(entries, entry.entry)
			return true
		}

		// For exact matches, stop if we found the one, otherwise continue looping
		if entry.prefix.Equal(prefix) {
			entries = append(entries, entry.entry)
			return false
		}

		return true
	})

	return entries, nil
}

func (m *Mem) Begin(ctx context.Context) (Transaction, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return &MemTransaction{
		mem:   m,
		start: time.Now(),
	}, nil
}

type MemTransaction struct {
	mu        sync.Mutex
	committed bool

	mem     *Mem
	changes []change

	start time.Time
}

type changekind uint8

const (
	changekindStore changekind = iota
	changekindDelete
)

type change struct {
	entry mementry
	kind  changekind
}

// Store implements Transaction.
func (m *MemTransaction) Store(ctx context.Context, entry Entry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.committed {
		return ErrCommitted
	}

	c := change{
		kind:  changekindStore,
		entry: newmementry(entry),
	}
	m.changes = append(m.changes, c)

	return nil
}

// Delete implements Transaction.
func (m *MemTransaction) Delete(ctx context.Context, dn DN) (err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.committed {
		return ErrCommitted
	}

	c := change{
		kind:  changekindDelete,
		entry: mementryFromPrefix(dn.Prefix()),
	}
	m.changes = append(m.changes, c)

	return nil
}

func (m *MemTransaction) Commit(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// lock transaction to read all changes and cleanup changes
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.committed {
		return ErrCommitted
	}

	// lock btree for writing
	m.mem.mu.Lock()
	defer m.mem.mu.Unlock()

	for _, change := range m.changes {
		switch change.kind {
		case changekindStore:
			m.mem.tr.ReplaceOrInsert(change.entry)
		case changekindDelete:
			m.mem.tr.Delete(change.entry)
		}
	}

	m.committed = true

	return nil
}

// Run implements run.Runnable
//
// This is a no-op.
func (m *Mem) Run(ctx context.Context, notify run.Notify) error {
	notify.Notify()
	return nil
}

// Metrics implements observability.Metrics.
func (m *Mem) Metrics() []prometheus.Collector {
	return []prometheus.Collector{m.metrics.backend}
}

// ReadinessChecks implements run.ReadinessChecks
//
// This is a no-op.
func (m *Mem) ReadinessChecks() map[string]observability.Check {
	return map[string]observability.Check{}
}
