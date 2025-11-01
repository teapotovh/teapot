package store

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	btree "github.com/google/btree"
)

type mementry struct {
	prefix Prefix
	entry  Entry
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
	mu sync.RWMutex
	tr *btree.BTreeG[mementry]
}

func NewMem() *Mem {
	return &Mem{
		tr: btree.NewG(2, mementryLess),
	}
}

// List implements Store.List
func (m *Mem) List(ctx context.Context, prefix Prefix, exact bool) ([]Entry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	start := mementryFromPrefix(prefix)
	end := mementryFromPrefix(prefixEnd(prefix))
	collected := []Entry{}
	m.tr.AscendRange(start, end, func(entry mementry) bool {
		// For non-exact matches, continue looping and collect all results
		if !exact {
			collected = append(collected, entry.entry)
			return true
		}

		// For exact matches, stop if we found the one, otherwise continue looping
		if entry.prefix.Equal(prefix) {
			collected = append(collected, entry.entry)
			return false
		}
		return true
	})

	return collected, nil
}

func (m *Mem) Begin(ctx context.Context) (Transaction, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return &MemTransaction{
		ctx: ctx,
		mem: m,
	}, nil
}

type MemTransaction struct {
	ctx context.Context
	mem *Mem

	mu      sync.Mutex
	changes []change
}

func (m *MemTransaction) Context() context.Context {
	return m.ctx
}

type changekind uint8

const (
	changekindStore changekind = iota
	changekindDelete
)

type change struct {
	kind  changekind
	entry mementry
}

// Store implements Store.Store
func (m *MemTransaction) Store(entry Entry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	c := change{
		kind:  changekindStore,
		entry: newmementry(entry),
	}
	m.changes = append(m.changes, c)
	return nil
}

// Delete implements Store.Delete
func (m *MemTransaction) Delete(dn DN) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	c := change{
		kind:  changekindDelete,
		entry: mementryFromPrefix(dn.Prefix()),
	}
	m.changes = append(m.changes, c)
	return nil
}

func (m *MemTransaction) Commit() error {
	if err := m.ctx.Err(); err != nil {
		return err
	}

	// lock btree for writing
	m.mem.mu.Lock()
	defer m.mem.mu.Unlock()

	// lock transaction to read all changes and cleanup changes
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, change := range m.changes {
		switch change.kind {
		case changekindStore:
			m.mem.tr.ReplaceOrInsert(change.entry)
		case changekindDelete:
			m.mem.tr.Delete(change.entry)
		}
	}

	m.changes = m.changes[:0]
	return nil
}
