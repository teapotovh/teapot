package pagedbtree

import (
	"container/list"
	"sync"
)

type pageCache struct {
	mu     sync.Mutex
	budget int64
	used   int64
	pages  map[uint64]*page
	order  *list.List
	hand   *list.Element
}

func newPageCache(budget int64) *pageCache {
	return &pageCache{
		budget: budget,
		pages:  make(map[uint64]*page),
		order:  list.New(),
	}
}

func (c *pageCache) get(id uint64) *page {
	c.mu.Lock()
	defer c.mu.Unlock()

	p, ok := c.pages[id]
	if !ok {
		return nil
	}

	p.used.Store(true)

	return p
}

func (c *pageCache) insertIfAbsent(p *page) *page {
	c.mu.Lock()
	defer c.mu.Unlock()

	if old, ok := c.pages[p.id]; ok {
		return old
	}

	c.addLocked(p)

	return p
}

func (c *pageCache) replace(p *page) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if old, ok := c.pages[p.id]; ok {
		c.removeLocked(old)
	}

	c.addLocked(p)
}

func (c *pageCache) remove(id uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if p, ok := c.pages[id]; ok {
		c.removeLocked(p)
	}
}

func (c *pageCache) dirtyPages() []*page {
	c.mu.Lock()
	defer c.mu.Unlock()

	pages := make([]*page, 0, len(c.pages))

	for _, p := range c.pages {
		if p.dirty.Load() {
			pages = append(pages, p)
		}
	}

	return pages
}

func (c *pageCache) markClean(p *page) {
	p.dirty.Store(false)
}

func (c *pageCache) addLocked(p *page) {
	p.elem = c.order.PushBack(p)
	c.pages[p.id] = p

	c.used += int64(len(p.data))
	if c.hand == nil {
		c.hand = p.elem
	}

	c.evictLocked()
}

func (c *pageCache) removeLocked(p *page) {
	delete(c.pages, p.id)
	c.used -= int64(len(p.data))

	if p.elem == nil {
		return
	}

	if c.hand == p.elem {
		c.hand = p.elem.Next()
	}

	c.order.Remove(p.elem)
	p.elem = nil
}

func (c *pageCache) evictLocked() {
	attempts := c.order.Len()*2 + 1

	for c.used > c.budget && attempts > 0 {
		attempts--

		if c.hand == nil {
			c.hand = c.order.Front()
		}

		if c.hand == nil {
			return
		}

		p, ok := c.hand.Value.(*page)
		if !ok {
			c.hand = c.hand.Next()

			continue
		}

		next := c.hand.Next()
		if next == nil {
			next = c.order.Front()
		}

		if p.used.Load() {
			p.used.Store(false)

			c.hand = next

			continue
		}

		if p.dirty.Load() {
			c.hand = next

			continue
		}

		c.removeLocked(p)
		c.hand = next
	}
}
