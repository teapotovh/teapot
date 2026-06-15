package pgcache

import (
	"iter"
	"maps"
	"sync/atomic"

	"github.com/google/btree"
)

type snapshot[K Key[K], V any] struct {
	tree *btree.BTreeG[K]
	vals map[string]V
}

type SortedMap[K Key[K], V any] struct {
	ptr atomic.Pointer[snapshot[K, V]]
}

func NewSortedMap[K Key[K], V any]() *SortedMap[K, V] {
	m := &SortedMap[K, V]{}
	m.ptr.Store(&snapshot[K, V]{
		tree: btree.NewG(32, func(a, b K) bool { return a.Less(b) }),
		vals: make(map[string]V),
	})

	return m
}

func (m *SortedMap[K, V]) copy() *snapshot[K, V] {
	old := m.ptr.Load()
	vals := make(map[string]V, len(old.vals))
	maps.Copy(vals, old.vals)

	return &snapshot[K, V]{tree: old.tree.Clone(), vals: vals}
}

func (m *SortedMap[K, V]) Store(k K, v V) {
	s := m.copy()
	s.tree.ReplaceOrInsert(k)
	s.vals[k.String()] = v
	m.ptr.Store(s)
}

func (m *SortedMap[K, V]) Delete(k K) {
	s := m.copy()
	s.tree.Delete(k)
	delete(s.vals, k.String())
	m.ptr.Store(s)
}

func (m *SortedMap[K, V]) Load(k K) (V, bool) {
	v, ok := m.ptr.Load().vals[k.String()]
	return v, ok
}

func (m *SortedMap[K, V]) All() iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		s := m.ptr.Load()
		s.tree.Ascend(func(k K) bool {
			return yield(k, s.vals[k.String()])
		})
	}
}

func (m *SortedMap[K, V]) From(from K) iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		s := m.ptr.Load()
		s.tree.AscendGreaterOrEqual(from, func(k K) bool {
			return yield(k, s.vals[k.String()])
		})
	}
}

func (m *SortedMap[K, V]) Between(from, to K) iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		s := m.ptr.Load()
		s.tree.AscendRange(from, to, func(k K) bool {
			return yield(k, s.vals[k.String()])
		})
	}
}
