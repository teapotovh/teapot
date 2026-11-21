package pagedbtree

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"math"
	"os"
	"slices"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/teapotovh/teapot/lib/run"
)

const maxDepth = 64

type unit = struct{}

var (
	// ErrInvalidConfig reports a missing or invalid configuration.
	ErrInvalidConfig = errors.New("invalid config")
	// ErrKeyTooLarge reports a key that does not fit in a page.
	ErrKeyTooLarge = errors.New("key too large")
	// ErrValueTooLarge reports a value that is too large for the file format.
	ErrValueTooLarge = errors.New("value too large")
	// ErrDiskFull reports that the configured disk size would be exceeded.
	ErrDiskFull = errors.New("disk size exceeded")
	// ErrDirtyPagesFull reports that the dirty page limit was reached.
	ErrDirtyPagesFull = errors.New("dirty page limit reached")
	// ErrNotRunning reports an operation before Run.
	ErrNotRunning = errors.New("store is not running")
)

// Config controls the store file and its memory budget.
type Config struct {
	// Path is the file used for the store.
	Path string
	// Memory is the page cache budget in bytes. Zero disables page caching.
	Memory int64
	// Disk is the maximum file size in bytes. Zero means no limit.
	Disk int64
	// DirtyPages is the maximum number of dirty pages. Zero means no limit.
	DirtyPages int
}

// Bytes is the byte-level B+tree store.
type Bytes struct {
	memory     int64
	disk       int64
	dirtyPages int

	path string
	fs   FS
	file File

	writeMu sync.Mutex
	metaMu  sync.Mutex

	pageCount  uint64
	freeHead   uint64
	pending    []uint64
	pendingSet map[uint64]struct{}

	root    atomic.Uint64
	readers atomic.Int64

	cache   *pageCache
	metrics metrics

	state atomic.Pointer[storeState]
	flush chan unit
}

type storeState struct {
	ctx    context.Context
	cancel context.CancelFunc
}

type pathEntry struct {
	id    uint64
	index int
}

type leafPage struct {
	id      uint64
	entries []leafEntry
}

// Open creates a byte-level store. The file is opened by Run.
func Open(fs FS, cfg Config) (*Bytes, error) {
	if fs == nil || cfg.Path == "" || cfg.Memory < 0 {
		return nil, ErrInvalidConfig
	}

	b := &Bytes{
		memory:     cfg.Memory,
		disk:       cfg.Disk,
		dirtyPages: cfg.DirtyPages,
		path:       cfg.Path,
		fs:         fs,
		cache:      newPageCache(cfg.Memory),
		pendingSet: make(map[uint64]struct{}),
		flush:      make(chan unit, 1),
	}
	b.metrics.init(b.path)

	return b, nil
}

// Load returns the value for key.
func (b *Bytes) Load(key string) ([]byte, bool, error) {
	start := time.Now()
	defer b.metrics.observeDuration(metricsOperationLoad, start)

	if err := b.contextErr(); err != nil {
		return nil, false, err
	}

	b.readers.Add(1)
	defer b.readers.Add(-1)

	root := b.root.Load()
	if root == 0 {
		return nil, false, nil
	}

	cur, err := b.seek(root, key)
	if err != nil {
		return nil, false, err
	}

	leaf, err := b.loadLeaf(cur.leaf)
	if err != nil {
		return nil, false, err
	}

	index := sort.Search(len(leaf.entries), func(i int) bool {
		return leaf.entries[i].key >= key
	})
	if index >= len(leaf.entries) || leaf.entries[index].key != key {
		return nil, false, nil
	}

	value, err := b.readEntryValue(leaf.entries[index])
	if err != nil {
		return nil, false, err
	}

	return value, true, nil
}

// Store writes a value for key.
func (b *Bytes) Store(key string, value []byte) error {
	start := time.Now()
	defer b.metrics.observeDuration(metricsOperationStore, start)

	if len(key) > maxKeyLength {
		return ErrKeyTooLarge
	}

	b.writeMu.Lock()
	defer b.writeMu.Unlock()

	err := b.applyStore(key, value)
	b.signalFlush()

	if err != nil {
		return err
	}

	b.metrics.observeEntrySize(len(key) + len(value))

	return nil
}

// All iterates over all entries in key order.
func (b *Bytes) All() iter.Seq2[string, []byte] {
	return b.scan("", "", false, metricsOperationAll)
}

// From iterates over entries with a key greater than or equal to from.
func (b *Bytes) From(from string) iter.Seq2[string, []byte] {
	return b.scan(from, "", false, metricsOperationFrom)
}

// Between iterates over entries with a key in [from, to).
func (b *Bytes) Between(from, to string) iter.Seq2[string, []byte] {
	return b.scan(from, to, true, metricsOperationBetween)
}

// Run implements run.Runnable.
func (b *Bytes) Run(ctx context.Context, notify run.Notify) (err error) {
	stateCtx, cancel := context.WithCancel(context.Background())
	state := &storeState{ctx: stateCtx, cancel: cancel}

	defer func() {
		cancel()

		b.waitReaders()

		if b.file != nil {
			err = errors.Join(err, b.file.Close())
			b.file = nil
		}
	}()

	b.file, err = b.fs.OpenFile(b.path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}

	if err := b.load(); err != nil {
		return err
	}

	b.state.Store(state)
	b.countEntries()
	notify.Notify()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-stateCtx.Done():
			return nil
		case <-b.flush:
			if err := b.flushNow(); err != nil {
				return err
			}
		}
	}
}

// Delete removes key from the store.
func (b *Bytes) Delete(key string) error {
	start := time.Now()
	defer b.metrics.observeDuration(metricsOperationDelete, start)

	if err := b.contextErr(); err != nil {
		return err
	}

	b.writeMu.Lock()
	defer b.writeMu.Unlock()

	err := b.applyDelete(key)
	b.signalFlush()

	return err
}

// Wait blocks until all previous writes are applied.
func (b *Bytes) Wait() error {
	return b.wait()
}

func (b *Bytes) applyStore(key string, value []byte) error {
	if err := b.contextErr(); err != nil {
		return err
	}

	root := b.root.Load()
	if root == 0 {
		return b.storeFirst(key, value)
	}

	return b.storeExisting(root, key, value)
}

func (b *Bytes) signalFlush() {
	select {
	case b.flush <- unit{}:
	default:
	}
}

// flushNow writes all dirty pages and the header, then returns.
func (b *Bytes) flushNow() error {
	b.writeMu.Lock()
	defer b.writeMu.Unlock()

	start := time.Now()
	defer b.metrics.observeDuration(metricsOperationWriteback, start)

	return b.flushLocked()
}

func (b *Bytes) applyDelete(key string) error {
	if err := b.contextErr(); err != nil {
		return err
	}

	root := b.root.Load()
	if root == 0 {
		return nil
	}

	cur, err := b.seek(root, key)
	if err != nil {
		return err
	}

	leaf, err := b.loadLeaf(cur.leaf)
	if err != nil {
		return err
	}

	index := sort.Search(len(leaf.entries), func(i int) bool {
		return leaf.entries[i].key >= key
	})
	if index >= len(leaf.entries) || leaf.entries[index].key != key {
		return nil
	}

	oldFirstKey := ""
	if len(leaf.entries) > 0 {
		oldFirstKey = leaf.entries[0].key
	}

	oldOverflow, err := b.overflowChain(leaf.entries[index].overflow)
	if err != nil {
		return err
	}

	entries := append(leaf.entries[:index], leaf.entries[index+1:]...)

	leftID, rightID, leftFirst, rightFirst, err := b.buildLeaf(entries)
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		leftFirst = oldFirstKey
	}

	b.addPendingFree(cur.leaf)
	b.addPendingFree(oldOverflow...)

	newRoot, err := b.copyUp(cur.path, leftID, leftFirst, rightID, rightFirst, true)
	if err != nil {
		b.discardPending()

		return err
	}

	if err := b.commitRoot(newRoot); err != nil {
		return err
	}

	b.metrics.decEntries()

	return nil
}

func (b *Bytes) contextErr() error {
	_, err := b.stateOrErr()
	return err
}

func (b *Bytes) stateOrErr() (*storeState, error) {
	state := b.state.Load()
	if state == nil {
		return nil, ErrNotRunning
	}

	if err := state.ctx.Err(); err != nil {
		return nil, err
	}

	return state, nil
}

func (b *Bytes) commitRoot(root uint64) error {
	if err := b.contextErr(); err != nil {
		b.discardPending()

		return err
	}

	if err := b.checkDirtyLimit(); err != nil {
		b.discardPending()

		return err
	}

	b.root.Store(root)

	return b.reclaimPending()
}

func (b *Bytes) checkDirtyLimit() error {
	if b.dirtyPages <= 0 {
		return nil
	}

	if b.cache.dirtyCount() > b.dirtyPages {
		return ErrDirtyPagesFull
	}

	return nil
}

func (b *Bytes) discardPending() {
	b.pending = nil
	b.pendingSet = make(map[uint64]struct{})
}

// All iterates over all entries in key order.
func (b *Bytes) storeFirst(key string, value []byte) error {
	entry, err := b.makeEntry(key, value)
	if err != nil {
		return err
	}

	root, err := b.buildLeafRoot([]leafEntry{entry})
	if err != nil {
		return err
	}

	if err := b.commitRoot(root); err != nil {
		return err
	}

	b.metrics.incEntries()

	return nil
}

func (b *Bytes) storeExisting(root uint64, key string, value []byte) error {
	cur, err := b.seek(root, key)
	if err != nil {
		return err
	}

	leaf, err := b.loadLeaf(cur.leaf)
	if err != nil {
		return err
	}

	index := sort.Search(len(leaf.entries), func(i int) bool {
		return leaf.entries[i].key >= key
	})

	exists := index < len(leaf.entries) && leaf.entries[index].key == key

	entries := leaf.entries
	oldOverflow := []uint64(nil)

	if exists {
		oldOverflow, err = b.overflowChain(entries[index].overflow)
		if err != nil {
			return err
		}

		entry, err := b.makeEntry(key, value)
		if err != nil {
			return err
		}

		entries[index] = entry
	} else {
		entry, err := b.makeEntry(key, value)
		if err != nil {
			return err
		}

		entries = append(entries, leafEntry{})
		copy(entries[index+1:], entries[index:])
		entries[index] = entry
	}

	leftID, rightID, leftFirst, rightFirst, err := b.buildLeaf(entries)
	if err != nil {
		return err
	}

	b.addPendingFree(cur.leaf)
	b.addPendingFree(oldOverflow...)

	newRoot, err := b.copyUp(cur.path, leftID, leftFirst, rightID, rightFirst, true)
	if err != nil {
		b.discardPending()

		return err
	}

	if err := b.commitRoot(newRoot); err != nil {
		return err
	}

	if !exists {
		b.metrics.incEntries()
	}

	return nil
}

func (b *Bytes) load() error {
	info, err := b.file.Stat()
	if err != nil {
		return fmt.Errorf("could not stat store file: %w", err)
	}

	if info.Size() < pageSize {
		return b.reset()
	}

	data := make([]byte, pageSize)
	if _, err := b.file.ReadAt(data, 0); err != nil {
		return b.reset()
	}

	header, err := decodeHeader(data)
	if err != nil ||
		header.Magic != fileMagic ||
		header.Version != fileVersion ||
		header.PageSize != pageSize ||
		header.Root > header.PageCount ||
		header.FreeHead > header.PageCount ||
		header.PageCount+1 > uint64(info.Size()/pageSize) { //nolint:gosec // the file size is not negative.
		return b.reset()
	}

	b.metaMu.Lock()
	b.pageCount = header.PageCount
	b.freeHead = header.FreeHead
	b.metaMu.Unlock()
	b.root.Store(header.Root)

	if header.Root != 0 {
		if _, err := b.loadPage(header.Root); err != nil {
			return b.reset()
		}
	}

	return nil
}

func (b *Bytes) reset() error {
	b.cache = newPageCache(b.memory)
	b.root.Store(0)
	b.metrics.setEntries(0)
	b.metrics.setCachedEntries(0)

	b.metaMu.Lock()
	b.pageCount = 0
	b.freeHead = 0
	b.pending = nil
	b.pendingSet = make(map[uint64]struct{})
	b.metaMu.Unlock()

	if err := b.file.Truncate(pageSize); err != nil {
		return fmt.Errorf("could not truncate store file: %w", err)
	}

	return b.writeHeader()
}

func (b *Bytes) writeHeader() error {
	b.metaMu.Lock()
	header := fileHeader{
		Magic:     fileMagic,
		Version:   fileVersion,
		PageSize:  pageSize,
		PageCount: b.pageCount,
		Root:      b.root.Load(),
		FreeHead:  b.freeHead,
	}
	b.metaMu.Unlock()

	if _, err := b.file.WriteAt(encodeHeader(header), 0); err != nil {
		return fmt.Errorf("could not write store header: %w", err)
	}

	return nil
}

func (b *Bytes) readPageFromDisk(id uint64) ([]byte, error) {
	b.metaMu.Lock()
	count := b.pageCount
	b.metaMu.Unlock()

	if id == 0 || id > count {
		return nil, fmt.Errorf("page %d out of range: %w", id, ErrCorrupt)
	}

	data := make([]byte, pageSize)
	if _, err := b.file.ReadAt(data, int64(id)*pageSize); err != nil { //nolint:gosec // fits in int64.
		return nil, fmt.Errorf("could not read page %d: %w", id, ErrCorrupt)
	}

	if _, err := parsePage(data); err != nil {
		return nil, fmt.Errorf("could not parse page %d: %w", id, err)
	}

	return data, nil
}

func (b *Bytes) loadPage(id uint64) (*page, error) {
	if p := b.cache.get(id); p != nil {
		return p, nil
	}

	data, err := b.readPageFromDisk(id)
	if err != nil {
		return nil, err
	}

	p := &page{id: id, data: data}
	p.used.Store(true)

	p = b.cache.insertIfAbsent(p)
	b.metrics.setCachedEntries(b.cache.entryCount())

	return p, nil
}

func (b *Bytes) writePage(p *page) error {
	if !p.dirty.Load() {
		return nil
	}

	if _, err := b.file.WriteAt(p.data, int64(p.id)*pageSize); err != nil { //nolint:gosec // fits in int64.
		return fmt.Errorf("could not write page %d: %w", p.id, err)
	}

	b.cache.markClean(p)

	return nil
}

func (b *Bytes) allocatePage() (uint64, error) {
	b.metaMu.Lock()
	id := b.freeHead
	b.metaMu.Unlock()

	if id != 0 {
		p, err := b.loadPage(id)
		if err != nil {
			return 0, err
		}

		header, err := parsePage(p.data)
		if err != nil {
			return 0, err
		}

		if header.typ == pageFree {
			b.metaMu.Lock()
			b.freeHead = header.next
			b.metaMu.Unlock()

			b.cache.remove(id)
			b.metrics.setCachedEntries(b.cache.entryCount())

			return id, nil
		}

		b.metaMu.Lock()
		b.freeHead = 0
		b.metaMu.Unlock()
	}

	b.metaMu.Lock()
	defer b.metaMu.Unlock()

	id = b.pageCount + 1
	if b.disk > 0 && int64(id+1)*pageSize > b.disk { //nolint:gosec // fits in int64.
		return 0, ErrDiskFull
	}

	b.pageCount = id

	return id, nil
}

func (b *Bytes) putPage(p *page) {
	p.dirty.Store(true)
	b.cache.replace(p)
	b.metrics.setCachedEntries(b.cache.entryCount())
}

func (b *Bytes) addPendingFree(ids ...uint64) {
	for _, id := range ids {
		if id == 0 {
			continue
		}

		if _, ok := b.pendingSet[id]; ok {
			continue
		}

		b.pendingSet[id] = struct{}{}
		b.pending = append(b.pending, id)
	}
}

func (b *Bytes) reclaimPending() error {
	if len(b.pending) == 0 || b.readers.Load() != 0 {
		return nil
	}

	pending := b.pending
	b.pending = nil
	b.pendingSet = make(map[uint64]struct{})

	for _, id := range pending {
		b.metaMu.Lock()
		next := b.freeHead
		b.metaMu.Unlock()

		p, err := makePage(id, pageFree, nil, next, 0)
		if err != nil {
			return err
		}

		b.putPage(p)

		b.metaMu.Lock()
		b.freeHead = id
		b.metaMu.Unlock()
	}

	return nil
}

func (b *Bytes) waitReaders() {
	for b.readers.Load() != 0 {
		time.Sleep(100 * time.Microsecond)
	}
}

func (b *Bytes) flushLocked() error {
	for _, p := range b.cache.dirtyPages() {
		if err := b.writePage(p); err != nil {
			return err
		}
	}

	return b.writeHeader()
}

func (b *Bytes) makeEntry(key string, value []byte) (leafEntry, error) {
	if uint64(len(value)) > math.MaxUint32 {
		return leafEntry{}, ErrValueTooLarge
	}

	entry := leafEntry{key: key, size: toUint32(len(value))}

	if len(value) <= maxInlineValue && leafEntryHeaderSize+len(key)+len(value) <= pagePayloadSize {
		entry.value = append([]byte(nil), value...)

		return entry, nil
	}

	first, err := b.writeOverflow(value)
	if err != nil {
		return leafEntry{}, err
	}

	entry.overflow = first

	return entry, nil
}

func (b *Bytes) writeOverflow(value []byte) (uint64, error) {
	if err := b.contextErr(); err != nil {
		return 0, err
	}

	chunks := (len(value) + pagePayloadSize - 1) / pagePayloadSize
	ids := make([]uint64, chunks)

	for i := range ids {
		if err := b.contextErr(); err != nil {
			return 0, err
		}

		id, err := b.allocatePage()
		if err != nil {
			return 0, err
		}

		ids[i] = id
	}

	for i, id := range ids {
		if err := b.contextErr(); err != nil {
			return 0, err
		}

		start := i * pagePayloadSize
		end := min(start+pagePayloadSize, len(value))

		next := uint64(0)
		if i+1 < len(ids) {
			next = ids[i+1]
		}

		p, err := makePage(id, pageOverflow, value[start:end], next, toUint16(end-start))
		if err != nil {
			return 0, err
		}

		b.putPage(p)
	}

	return ids[0], nil
}

func (b *Bytes) readEntryValue(entry leafEntry) ([]byte, error) {
	if entry.overflow == 0 {
		return append([]byte(nil), entry.value...), nil
	}

	value := make([]byte, entry.size)
	offset := 0
	id := entry.overflow

	for id != 0 {
		if err := b.contextErr(); err != nil {
			return nil, err
		}

		p, err := b.loadPage(id)
		if err != nil {
			return nil, err
		}

		header, err := parsePage(p.data)
		if err != nil {
			return nil, err
		}

		if header.typ != pageOverflow {
			return nil, fmt.Errorf("expected overflow page: %w", ErrCorrupt)
		}

		end := offset + int(header.count)
		if end > len(value) {
			return nil, fmt.Errorf("overflow value is too long: %w", ErrCorrupt)
		}

		copy(value[offset:end], p.data[pageHeaderSize:pageHeaderSize+int(header.count)])
		offset = end
		id = header.next
	}

	if offset != len(value) {
		return nil, fmt.Errorf("overflow value is too short: %w", ErrCorrupt)
	}

	return value, nil
}

func (b *Bytes) overflowChain(first uint64) ([]uint64, error) {
	if err := b.contextErr(); err != nil {
		return nil, err
	}

	ids := []uint64{}

	for id := first; id != 0; {
		if err := b.contextErr(); err != nil {
			return nil, err
		}

		ids = append(ids, id)

		if len(ids) > toInt(b.pageCount)+1 {
			return nil, fmt.Errorf("overflow chain is too long: %w", ErrCorrupt)
		}

		p, err := b.loadPage(id)
		if err != nil {
			return nil, err
		}

		header, err := parsePage(p.data)
		if err != nil {
			return nil, err
		}

		if header.typ != pageOverflow {
			return nil, fmt.Errorf("expected overflow page: %w", ErrCorrupt)
		}

		id = header.next
	}

	return ids, nil
}

type cursor struct {
	store *Bytes
	path  []pathEntry
	leaf  uint64
}

func (b *Bytes) seek(root uint64, key string) (cursor, error) {
	if err := b.contextErr(); err != nil {
		return cursor{}, err
	}

	c := cursor{store: b}
	id := root

	for range maxDepth {
		if err := b.contextErr(); err != nil {
			return cursor{}, err
		}

		p, err := b.loadPage(id)
		if err != nil {
			return cursor{}, err
		}

		header, err := parsePage(p.data)
		if err != nil {
			return cursor{}, err
		}

		switch header.typ {
		case pageLeaf:
			c.leaf = id

			return c, nil
		case pageInternal:
			entries, err := decodeInternalEntries(p.data[pageHeaderSize:], header.count)
			if err != nil {
				return cursor{}, err
			}

			if len(entries) == 0 {
				return cursor{}, fmt.Errorf("empty internal page: %w", ErrCorrupt)
			}

			index := max(sort.Search(len(entries), func(i int) bool {
				return entries[i].key > key
			})-1, 0)

			c.path = append(c.path, pathEntry{id: id, index: index})
			id = entries[index].child
		case pageFree, pageOverflow:
			return cursor{}, fmt.Errorf("unexpected page type %d: %w", header.typ, ErrCorrupt)
		default:
			return cursor{}, fmt.Errorf("unexpected page type %d: %w", header.typ, ErrCorrupt)
		}
	}

	return cursor{}, fmt.Errorf("tree is too deep: %w", ErrCorrupt)
}

func (c *cursor) next() (bool, error) {
	for i := len(c.path) - 1; i >= 0; i-- {
		if err := c.store.contextErr(); err != nil {
			return false, err
		}

		p, err := c.store.loadPage(c.path[i].id)
		if err != nil {
			return false, err
		}

		header, err := parsePage(p.data)
		if err != nil {
			return false, err
		}

		if header.typ != pageInternal {
			return false, fmt.Errorf("expected internal page: %w", ErrCorrupt)
		}

		entries, err := decodeInternalEntries(p.data[pageHeaderSize:], header.count)
		if err != nil {
			return false, err
		}

		if c.path[i].index+1 >= len(entries) {
			continue
		}

		c.path = c.path[:i+1]
		c.path[i].index++

		child := entries[c.path[i].index].child

		for range maxDepth {
			p, err := c.store.loadPage(child)
			if err != nil {
				return false, err
			}

			header, err := parsePage(p.data)
			if err != nil {
				return false, err
			}

			if header.typ == pageLeaf {
				c.leaf = child

				return true, nil
			}

			if header.typ != pageInternal {
				return false, fmt.Errorf("unexpected page type %d: %w", header.typ, ErrCorrupt)
			}

			entries, err := decodeInternalEntries(p.data[pageHeaderSize:], header.count)
			if err != nil {
				return false, err
			}

			if len(entries) == 0 {
				return false, fmt.Errorf("empty internal page: %w", ErrCorrupt)
			}

			c.path = append(c.path, pathEntry{id: child, index: 0})
			child = entries[0].child
		}

		return false, fmt.Errorf("tree is too deep: %w", ErrCorrupt)
	}

	c.leaf = 0

	return false, nil
}

func (b *Bytes) loadLeaf(id uint64) (leafPage, error) {
	p, err := b.loadPage(id)
	if err != nil {
		return leafPage{}, err
	}

	header, err := parsePage(p.data)
	if err != nil {
		return leafPage{}, err
	}

	if header.typ != pageLeaf {
		return leafPage{}, fmt.Errorf("expected leaf page: %w", ErrCorrupt)
	}

	entries, err := decodeLeafEntries(p.data[pageHeaderSize:], header.count)
	if err != nil {
		return leafPage{}, err
	}

	return leafPage{id: id, entries: entries}, nil
}

func (b *Bytes) buildLeafRoot(entries []leafEntry) (uint64, error) {
	leftID, rightID, leftFirst, rightFirst, err := b.buildLeaf(entries)
	if err != nil {
		return 0, err
	}

	if rightID == 0 {
		return leftID, nil
	}

	return b.makeRoot(leftID, leftFirst, rightID, rightFirst)
}

func (b *Bytes) buildLeaf(entries []leafEntry) (uint64, uint64, string, string, error) {
	total := leafEntriesSize(entries)

	if total <= pagePayloadSize {
		id, err := b.allocatePage()
		if err != nil {
			return 0, 0, "", "", err
		}

		p, err := makePage(id, pageLeaf, encodeLeafEntries(entries), 0, toUint16(len(entries)))
		if err != nil {
			return 0, 0, "", "", err
		}

		b.putPage(p)

		first := ""
		if len(entries) > 0 {
			first = entries[0].key
		}

		return id, 0, first, "", nil
	}

	split := splitLeafIndex(entries)
	if split < 1 {
		return 0, 0, "", "", ErrEntryTooLarge
	}

	leftEntries := entries[:split]
	rightEntries := entries[split:]

	leftID, err := b.allocatePage()
	if err != nil {
		return 0, 0, "", "", err
	}

	rightID, err := b.allocatePage()
	if err != nil {
		return 0, 0, "", "", err
	}

	left, err := makePage(
		leftID,
		pageLeaf,
		encodeLeafEntries(leftEntries),
		0,
		toUint16(len(leftEntries)),
	)
	if err != nil {
		return 0, 0, "", "", err
	}

	right, err := makePage(
		rightID,
		pageLeaf,
		encodeLeafEntries(rightEntries),
		0,
		toUint16(len(rightEntries)),
	)
	if err != nil {
		return 0, 0, "", "", err
	}

	b.putPage(left)
	b.putPage(right)

	return leftID, rightID, leftEntries[0].key, rightEntries[0].key, nil
}

func splitLeafIndex(entries []leafEntry) int {
	total := leafEntriesSize(entries)
	left := 0

	for i := 1; i < len(entries); i++ {
		left += leafEntrySize(entries[i-1])
		right := total - left

		if left <= pagePayloadSize && right <= pagePayloadSize {
			return i
		}
	}

	return -1
}

func (b *Bytes) buildInternal(entries []internalEntry) (uint64, uint64, string, string, error) {
	total := internalEntriesSize(entries)

	if total <= pagePayloadSize {
		id, err := b.allocatePage()
		if err != nil {
			return 0, 0, "", "", err
		}

		p, err := makePage(id, pageInternal, encodeInternalEntries(entries), 0, toUint16(len(entries)))
		if err != nil {
			return 0, 0, "", "", err
		}

		b.putPage(p)

		first := ""
		if len(entries) > 0 {
			first = entries[0].key
		}

		return id, 0, first, "", nil
	}

	split := splitInternalIndex(entries)
	if split < 1 {
		return 0, 0, "", "", ErrEntryTooLarge
	}

	leftEntries := entries[:split]
	rightEntries := entries[split:]

	leftID, err := b.allocatePage()
	if err != nil {
		return 0, 0, "", "", err
	}

	rightID, err := b.allocatePage()
	if err != nil {
		return 0, 0, "", "", err
	}

	left, err := makePage(
		leftID,
		pageInternal,
		encodeInternalEntries(leftEntries),
		0,
		toUint16(len(leftEntries)),
	)
	if err != nil {
		return 0, 0, "", "", err
	}

	right, err := makePage(
		rightID,
		pageInternal,
		encodeInternalEntries(rightEntries),
		0,
		toUint16(len(rightEntries)),
	)
	if err != nil {
		return 0, 0, "", "", err
	}

	b.putPage(left)
	b.putPage(right)

	return leftID, rightID, leftEntries[0].key, rightEntries[0].key, nil
}

func internalEntriesSize(entries []internalEntry) int {
	size := 0
	for _, entry := range entries {
		size += internalEntryHeaderSize + len(entry.key)
	}

	return size
}

func splitInternalIndex(entries []internalEntry) int {
	total := internalEntriesSize(entries)
	left := 0

	for i := 1; i < len(entries); i++ {
		left += internalEntryHeaderSize + len(entries[i-1].key)
		right := total - left

		if left <= pagePayloadSize && right <= pagePayloadSize {
			return i
		}
	}

	return -1
}

func (b *Bytes) makeRoot(leftID uint64, leftFirst string, rightID uint64, rightFirst string) (uint64, error) {
	entries := []internalEntry{
		{child: leftID, key: leftFirst},
		{child: rightID, key: rightFirst},
	}

	for range maxDepth {
		id, newRight, first, newRightFirst, err := b.buildInternal(entries)
		if err != nil {
			return 0, err
		}

		if newRight == 0 {
			return id, nil
		}

		entries = []internalEntry{
			{child: id, key: first},
			{child: newRight, key: newRightFirst},
		}
	}

	return 0, fmt.Errorf("tree is too deep: %w", ErrCorrupt)
}

func (b *Bytes) copyUp(
	path []pathEntry,
	childID uint64,
	childFirst string,
	rightID uint64,
	rightFirst string,
	updateKey bool,
) (uint64, error) {
	for _, entry := range slices.Backward(path) {
		p, err := b.loadPage(entry.id)
		if err != nil {
			return 0, err
		}

		header, err := parsePage(p.data)
		if err != nil {
			return 0, err
		}

		entries, err := decodeInternalEntries(p.data[pageHeaderSize:], header.count)
		if err != nil {
			return 0, err
		}

		if entry.index < 0 || entry.index >= len(entries) {
			return 0, fmt.Errorf("child index out of range: %w", ErrCorrupt)
		}

		entries[entry.index].child = childID
		if updateKey {
			entries[entry.index].key = childFirst
		}

		if rightID != 0 {
			entries = append(entries, internalEntry{})
			copy(entries[entry.index+2:], entries[entry.index+1:])
			entries[entry.index+1] = internalEntry{child: rightID, key: rightFirst}
			rightID = 0
		}

		b.addPendingFree(entry.id)

		newID, newRight, newFirst, newRightFirst, err := b.buildInternal(entries)
		if err != nil {
			return 0, err
		}

		childID = newID
		childFirst = newFirst

		if newRight != 0 {
			rightID = newRight
			rightFirst = newRightFirst
		}
	}

	if rightID != 0 {
		return b.makeRoot(childID, childFirst, rightID, rightFirst)
	}

	return childID, nil
}

func (b *Bytes) countEntries() {
	root := b.root.Load()
	if root == 0 {
		b.metrics.setEntries(0)

		return
	}

	cur, err := b.seek(root, "")
	if err != nil {
		return
	}

	var total int64

	for cur.leaf != 0 {
		leaf, err := b.loadLeaf(cur.leaf)
		if err != nil {
			break
		}

		total += int64(len(leaf.entries))

		ok, err := cur.next()
		if err != nil || !ok {
			break
		}
	}

	b.metrics.setEntries(total)
}

func (b *Bytes) scan(from, to string, bounded bool, operation string) iter.Seq2[string, []byte] {
	return func(yield func(string, []byte) bool) {
		start := time.Now()
		defer b.metrics.observeDuration(operation, start)

		if b.contextErr() != nil {
			return
		}

		b.readers.Add(1)
		defer b.readers.Add(-1)

		root := b.root.Load()
		if root == 0 {
			return
		}

		cur, err := b.seek(root, from)
		if err != nil {
			return
		}

		for cur.leaf != 0 {
			if b.contextErr() != nil {
				return
			}

			leaf, err := b.loadLeaf(cur.leaf)
			if err != nil {
				return
			}

			for _, entry := range leaf.entries {
				if entry.key < from {
					continue
				}

				if bounded && entry.key >= to {
					return
				}

				value, err := b.readEntryValue(entry)
				if err != nil {
					return
				}

				if !yield(entry.key, value) {
					return
				}
			}

			ok, err := cur.next()
			if err != nil || !ok {
				return
			}
		}
	}
}

// Ensure Bytes implements run.Runnable.
var _ run.Runnable = &Bytes{}
