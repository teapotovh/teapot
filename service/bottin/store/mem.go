package store

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/google/btree"

	"github.com/teapotovh/teapot/lib/observability"
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

// Get implements Store.
func (m *Mem) Get(ctx context.Context, prefix Prefix) (Entry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, found := m.tr.Get(mementryFromPrefix(prefix))
	if !found {
		return Entry{}, ErrNotFound
	}

	return entry.entry, nil
}

// List implements Store.
func (m *Mem) List(ctx context.Context, prefix Prefix) (entries []Entry, err error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	collect := func(entry mementry) bool {
		entries = append(entries, entry.entry)

		return true
	}

	// An empty prefix has no upper bound, so scan the whole tree.
	if len(prefix) == 0 {
		m.tr.Ascend(collect)

		return entries, nil
	}

	start := mementryFromPrefix(prefix)
	end := mementryFromPrefix(prefixEnd(prefix))

	m.tr.AscendRange(start, end, collect)

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

// Rollback implements Transaction.
func (m *MemTransaction) Rollback(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.committed {
		return ErrCommitted
	}

	m.committed = true
	m.changes = nil

	return nil
}

// ReadinessChecks implements observability.ReadinessChecks.
//
// This is a no-op.
func (m *Mem) ReadinessChecks() map[string]observability.Check {
	return map[string]observability.Check{}
}
