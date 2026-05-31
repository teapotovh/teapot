package store

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	btree "github.com/google/btree"
	"github.com/prometheus/client_golang/prometheus"
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

func prefixEnd(prefix Prefix) Prefix {
	if len(prefix) == 0 {
		return Prefix(nil)
	}

	lastComponent := prefix[len(prefix)-1]
	lastComponentValue := fmt.Sprintf("%s%c", lastComponent.Value, utf8.MaxRune)
	cpy := prefix.Clone()
	cpy[len(cpy)-1] = Component{
		Type:  lastComponent.Type,
		Value: lastComponentValue,
	}

	return cpy
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
	now := time.Now()
	defer func() {
		m.metrics.operationDuration.WithLabelValues(operationList, status(err)).Observe(time.Since(now).Seconds())
	}()

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
		ctx:   ctx,
		mem:   m,
		start: time.Now(),
	}, nil
}

type MemTransaction struct {
	ctx      context.Context
	mu       sync.Mutex
	commited bool

	mem     *Mem
	changes []change

	start time.Time
}

// Context implements Transaction
func (m *MemTransaction) Context() context.Context {
	return m.ctx
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
func (m *MemTransaction) Store(entry Entry) (err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	start := time.Now()
	defer func() {
		m.mem.metrics.operationDuration.WithLabelValues(operationStore, status(err)).Observe(time.Since(start).Seconds())
	}()

	if m.commited {
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
func (m *MemTransaction) Delete(dn DN) (err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	start := time.Now()
	defer func() {
		m.mem.metrics.operationDuration.WithLabelValues(operationDelete, status(err)).Observe(time.Since(start).Seconds())
	}()

	if m.commited {
		return ErrCommitted
	}

	c := change{
		kind:  changekindDelete,
		entry: mementryFromPrefix(dn.Prefix()),
	}
	m.changes = append(m.changes, c)

	return nil
}

func (m *MemTransaction) Commit() (err error) {
	if err := m.ctx.Err(); err != nil {
		return err
	}

	defer func() {
		m.mem.metrics.transactionDuration.WithLabelValues(strconv.Itoa(len(m.changes)), status(err)).Observe(time.Since(m.start).Seconds())
	}()

	// lock transaction to read all changes and cleanup changes
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.commited {
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

	m.commited = true
	return nil
}

// Metrics implements observability.Metrics.
func (m *Mem) Metrics() []prometheus.Collector {
	return []prometheus.Collector{
		m.metrics.backend,
		m.metrics.operationDuration,
		m.metrics.transactionDuration,
	}
}
