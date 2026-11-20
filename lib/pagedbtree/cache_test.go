package pagedbtree

import "testing"

func newTestPage(id uint64, dirty bool) *page {
	p := &page{id: id, data: make([]byte, pageSize)}
	p.dirty.Store(dirty)

	return p
}

func TestCacheEvictsCleanPages(t *testing.T) {
	t.Parallel()

	cache := newPageCache(pageSize)

	first := newTestPage(1, false)
	second := newTestPage(2, false)

	cache.insertIfAbsent(first)
	cache.insertIfAbsent(second)

	if cache.get(first.id) != nil {
		t.Fatalf("page %d was not evicted", first.id)
	}

	if cache.get(second.id) != second {
		t.Fatalf("page %d is not cached", second.id)
	}
}

func TestCacheKeepsDirtyPages(t *testing.T) {
	t.Parallel()

	cache := newPageCache(pageSize)

	first := newTestPage(1, true)
	second := newTestPage(2, false)

	cache.insertIfAbsent(first)
	cache.insertIfAbsent(second)

	if cache.get(first.id) != first {
		t.Fatalf("dirty page %d was evicted", first.id)
	}

	if cache.get(second.id) != nil {
		t.Fatalf("clean page %d was not evicted", second.id)
	}
}

func TestCacheReplace(t *testing.T) {
	t.Parallel()

	cache := newPageCache(pageSize)

	old := newTestPage(1, true)
	cache.insertIfAbsent(old)

	replacement := newTestPage(1, true)
	cache.replace(replacement)

	if cache.get(1) != replacement {
		t.Fatal("replace did not update the cached page")
	}
}
