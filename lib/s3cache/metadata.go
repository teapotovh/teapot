package s3cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

const v1MetadataFileName = "v1.metadata.json"

// entry describes one cached object.
type entry struct {
	ID       uuid.UUID `json:"id"`
	Key      string    `json:"key"`
	Hash     Hash      `json:"hash"`
	Size     int64     `json:"size"`
	StoredAt time.Time `json:"stored_at"`

	// lastAccess is tracked in memory only so cache hits stay cheap.
	// At startup, it is initialized based on the StoredAt field.
	lastAccess atomic.Int64
}

func (e *entry) touch() {
	e.lastAccess.Store(time.Now().UnixNano())
}

// snapshot is a point-in-time copy of an entry's fields, safe to
// read without holding any lock.
type snapshot struct {
	ID         uuid.UUID
	Key        string
	Hash       Hash
	Size       int64
	StoredAt   time.Time
	LastAccess time.Time
}

// metadataStore holds all cache metadata in memory, backed by a single
// JSON file on disk. Reads never touch disk; writes rewrite the whole
// file. That's fine here since writes only happen on cache misses
// (S3 fetches) and evictions -- not on every read.
type metadataStore struct {
	path string
	size atomic.Int64

	byKey map[string]*entry

	mu sync.RWMutex
}

func newMetadataStore(path string) (*metadataStore, error) {
	metadataPath := filepath.Join(path, v1MetadataFileName)
	ms := &metadataStore{
		path: metadataPath,
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ms, nil
		}

		return nil, fmt.Errorf("error while reading s3 metadata: %w", err)
	}

	var entries []*entry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("error while unmarshaling s3 metadata (v1): %w", err)
	}

	for _, e := range entries {
		e.lastAccess.Store(e.StoredAt.UnixNano())
		ms.byKey[e.Key] = e
	}

	return ms, nil
}

// persistLocked writes the full metadata set to disk. Caller must hold mu.
func (ms *metadataStore) persistLocked() error {
	ms.mu.RLock()
	entries := make([]*entry, 0, len(ms.byKey))
	for _, e := range ms.byKey {
		entries = append(entries, e)
	}
	ms.mu.RUnlock()

	data, err := json.Marshal(entries)
	if err != nil {
		return fmt.Errorf("error while marshaling metadata: %w", err)
	}

	ms.size.Store(int64(len(data)))
	return writeFileAtomic(ms.path, data)
}

func (ms *metadataStore) lockedGet(key string) (*entry, bool) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	e, ok := ms.byKey[key]
	return e, ok
}

// get returns a snapshot of the entry for key, if any.
func (ms *metadataStore) get(key string) *snapshot {
	e, ok := ms.lockedGet(key)
	if !ok {
		return nil
	}

	return &snapshot{
		ID:       e.ID,
		Key:      e.Key,
		Hash:     e.Hash,
		Size:     e.Size,
		StoredAt: e.StoredAt,
	}
}

// touch bumps the in-memory last-access time for key, if present.
func (ms *metadataStore) touch(key string) {
	e, ok := ms.lockedGet(key)

	if ok {
		e.touch()
	}
}

// resolveUID returns the UID a data file for key should use. If a key is being
// tracked, an existing one is returned. Also returns the entry's current size
// (0 if new), so callers can compute a byte-delta for accounting.
func (ms *metadataStore) resolveUID(key string) (uid uuid.UUID, oldSize int64) {
	e, ok := ms.lockedGet(key)
	if ok {
		return e.ID, e.Size
	}

	return uuid.New(), 0
}

// put records that key now lives at uid with the given hash/size, and
// persists the metadata file. uid should come from resolveUID, called
// before the data file itself was written.
func (ms *metadataStore) put(key string, id uuid.UUID, hash Hash, size int64) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	e, ok := ms.byKey[key]
	if !ok {
		e = &entry{ID: id, Key: key}
		ms.byKey[key] = e
	}
	e.Hash = hash
	e.Size = size
	e.StoredAt = time.Now()
	e.touch()
}

// remove deletes the entry for key, if present, and persists the change.
func (ms *metadataStore) remove(key string) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	_, ok := ms.byKey[key]
	if !ok {
		return
	}
	delete(ms.byKey, key)
}

// metadataSize returns the size of the metadata file.
func (ms *metadataStore) metadataSize() int64 {
	return ms.size.Load()
}

// cacheSize returns the sum of all tracked entry sizes.
func (ms *metadataStore) cacheSize() int64 {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	total := int64(0)
	for _, e := range ms.byKey {
		total += e.Size
	}
	return total
}

// count returns the number of tracked entries.
func (ms *metadataStore) count() int {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	return len(ms.byKey)
}

// lruHeap returns all entries as eviction candidates, least-recently -used first.
func (ms *metadataStore) lruHeap() []snapshot {
	ms.mu.RLock()
	out := make([]snapshot, 0, len(ms.byKey))
	for _, e := range ms.byKey {
		out = append(out, snapshot{
			ID:         e.ID,
			Key:        e.Key,
			Hash:       e.Hash,
			Size:       e.Size,
			StoredAt:   e.StoredAt,
			LastAccess: time.Unix(0, e.lastAccess.Load()),
		})
	}
	ms.mu.RUnlock()

	sort.Slice(out, func(i, j int) bool { return out[i].LastAccess.Before(out[j].LastAccess) })
	return out
}
