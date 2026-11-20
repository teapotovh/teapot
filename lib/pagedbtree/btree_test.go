package pagedbtree

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"
)

func testConfig(t *testing.T) Config {
	t.Helper()

	return Config{
		Path:   filepath.Join(t.TempDir(), "store.db"),
		Memory: 1 << 20,
		Disk:   32 << 20,
	}
}

func TestBytesRandomOperations(t *testing.T) {
	t.Parallel()
	cfg := testConfig(t)

	store, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}

	expected := map[string][]byte{}
	rng := rand.New(rand.NewSource(42)) //nolint:gosec

	for i := range 4000 {
		key := fmt.Sprintf("key-%04d", rng.Intn(500))

		switch op := rng.Intn(10); {
		case op < 6:
			var value []byte
			if rng.Intn(20) == 0 {
				value = bytes.Repeat([]byte{byte(i)}, pageSize+rng.Intn(pageSize))
			} else {
				value = []byte(fmt.Sprintf("value-%d-%d", i, rng.Intn(10000)))
			}

			if err := store.Store(key, value); err != nil {
				t.Fatal(err)
			}

			validateTree(t, store)

			expected[key] = value
		case op < 8:
			if err := store.Delete(key); err != nil {
				t.Fatal(err)
			}

			validateTree(t, store)

			delete(expected, key)
		default:
			got, ok, err := store.Load(key)
			if err != nil {
				t.Fatal(err)
			}

			want, exists := expected[key]
			if ok != exists {
				t.Fatalf("Load(%q) found=%v, want %v", key, ok, exists)
			}

			if ok && !bytes.Equal(got, want) {
				t.Fatalf("Load(%q) = %q, want %q", key, got, want)
			}
		}
	}

	checkContents(t, store, expected)

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = Open(cfg)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = store.Close() })

	checkContents(t, store, expected)
}

func TestBytesRangeScan(t *testing.T) {
	t.Parallel()
	cfg := testConfig(t)

	store, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = store.Close() })

	for _, key := range []string{"a", "b", "c", "d", "e"} {
		if err := store.Store(key, []byte("value-"+key)); err != nil {
			t.Fatal(err)
		}
	}

	collect := func(seq func(func(string, []byte) bool)) []string {
		keys := []string{}

		seq(func(key string, _ []byte) bool {
			keys = append(keys, key)
			return true
		})

		return keys
	}

	if got := collect(store.All()); fmt.Sprint(got) != "[a b c d e]" {
		t.Fatalf("All() = %v", got)
	}

	if got := collect(store.From("c")); fmt.Sprint(got) != "[c d e]" {
		t.Fatalf("From(c) = %v", got)
	}

	if got := collect(store.Between("b", "e")); fmt.Sprint(got) != "[b c d]" {
		t.Fatalf("Between(b, e) = %v", got)
	}
}

func TestCursorWalk(t *testing.T) {
	t.Parallel()
	cfg := testConfig(t)
	cfg.Memory = 4 << 20

	store, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = store.Close() })

	value := bytes.Repeat([]byte{7}, maxInlineValue)

	const count = 800

	for i := range count {
		if err := store.Store(fmt.Sprintf("key-%04d", i), value); err != nil {
			t.Fatal(err)
		}
	}

	cur, err := store.seek(store.root.Load(), "key-0000")
	if err != nil {
		t.Fatal(err)
	}

	if len(cur.path) < 2 {
		t.Fatalf("cursor path depth = %d, want at least 2", len(cur.path))
	}

	entries := 0

	for cur.leaf != 0 {
		leaf, err := store.loadLeaf(cur.leaf)
		if err != nil {
			t.Fatal(err)
		}

		entries += len(leaf.entries)

		ok, err := cur.next()
		if err != nil {
			t.Fatal(err)
		}

		if !ok {
			break
		}
	}

	if entries != count {
		t.Fatalf("cursor walked %d entries, want %d", entries, count)
	}
}

func TestBytesLargeValues(t *testing.T) {
	t.Parallel()
	cfg := testConfig(t)

	store, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = store.Close() })

	values := [][]byte{
		{},
		{1},
		bytes.Repeat([]byte{2}, maxInlineValue),
		bytes.Repeat([]byte{3}, maxInlineValue+1),
		bytes.Repeat([]byte{4}, pageSize*3+7),
	}

	for i, value := range values {
		key := fmt.Sprintf("large-%d", i)

		if err := store.Store(key, value); err != nil {
			t.Fatal(err)
		}

		got, ok, err := store.Load(key)
		if err != nil {
			t.Fatal(err)
		}

		if !ok || !bytes.Equal(got, value) {
			t.Fatalf("Load(%q) did not return the stored value", key)
		}
	}

	if err := store.Store("large-1", []byte("small")); err != nil {
		t.Fatal(err)
	}

	got, ok, err := store.Load("large-1")
	if err != nil {
		t.Fatal(err)
	}

	if !ok || !bytes.Equal(got, []byte("small")) {
		t.Fatalf("overwritten value = %q, want small", got)
	}

	if err := store.Delete("large-4"); err != nil {
		t.Fatal(err)
	}

	if _, ok, err := store.Load("large-4"); err != nil || ok {
		t.Fatalf("Load after Delete = ok %v, err %v", ok, err)
	}
}

func TestNoPageCache(t *testing.T) {
	t.Parallel()
	cfg := testConfig(t)
	cfg.Memory = 0

	store, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}

	for i := range 200 {
		key := fmt.Sprintf("key-%03d", i)
		if err := store.Store(key, []byte("value")); err != nil {
			t.Fatal(err)
		}
	}

	for i := range 200 {
		key := fmt.Sprintf("key-%03d", i)
		if _, ok, err := store.Load(key); err != nil || !ok {
			t.Fatalf("Load(%q) = ok %v, err %v", key, ok, err)
		}
	}

	count := 0
	for range store.All() {
		count++
	}

	if count != 200 {
		t.Fatalf("All() returned %d entries, want 200", count)
	}

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()

	if _, ok, err := reopened.Load("key-100"); err != nil || !ok {
		t.Fatalf("Load after reopen = ok %v, err %v", ok, err)
	}
}

func TestInvalidMemory(t *testing.T) {
	t.Parallel()
	cfg := testConfig(t)
	cfg.Memory = -1

	if _, err := Open(cfg); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("Open() error = %v, want %v", err, ErrInvalidConfig)
	}
}

func TestCorruptionWipe(t *testing.T) {
	t.Parallel()
	cfg := testConfig(t)

	store, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}

	for i := range 100 {
		if err := store.Store(fmt.Sprintf("key-%03d", i), []byte("value")); err != nil {
			t.Fatal(err)
		}
	}

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(cfg.Path)
	if err != nil {
		t.Fatal(err)
	}

	root := binary.LittleEndian.Uint64(data[24:])
	if root == 0 {
		t.Fatal("root page is zero")
	}

	offset := int(root)*pageSize + pageHeaderSize + 4 //nolint:gosec // the root page fits in an int.
	data[offset] ^= 0xff

	if err := os.WriteFile(cfg.Path, data, 0o600); err != nil { //nolint:gosec // the path is a test temporary file.
		t.Fatal(err)
	}

	store, err = Open(cfg)
	if err != nil {
		t.Fatal(err)
	}

	if _, ok, err := store.Load("key-000"); err != nil || ok {
		t.Fatalf("Load after corruption = ok %v, err %v", ok, err)
	}

	if err := store.Store("new", []byte("value")); err != nil {
		t.Fatal(err)
	}

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentAccess(t *testing.T) {
	t.Parallel()
	cfg := testConfig(t)
	cfg.Memory = 256 << 10

	store, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = store.Close() })

	const (
		writers   = 4
		perWriter = 200
	)

	var wg sync.WaitGroup

	errs := make(chan error, writers*2)

	for writer := range writers {
		wg.Go(func() {
			for i := range perWriter {
				key := fmt.Sprintf("w%d-%04d", writer, i)

				if err := store.Store(key, []byte("value")); err != nil {
					errs <- err
					return
				}
			}
		})
	}

	for range 2 {
		wg.Go(func() {
			for range 500 {
				for _, value := range store.All() {
					_ = value
				}
			}
		})
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Fatal(err)
	}

	for writer := range writers {
		for i := range perWriter {
			key := fmt.Sprintf("w%d-%04d", writer, i)
			if _, ok, err := store.Load(key); err != nil || !ok {
				t.Fatalf("Load(%q) = ok %v, err %v", key, ok, err)
			}
		}
	}
}

type testNotify struct {
	ch chan struct{}
}

func (n testNotify) Notify() {
	select {
	case n.ch <- struct{}{}:
	default:
	}
}

func TestRunPersists(t *testing.T) {
	t.Parallel()
	cfg := testConfig(t)

	store, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	done := make(chan error, 1)
	notify := testNotify{ch: make(chan struct{}, 1)}

	go func() {
		done <- store.Run(ctx, notify)
	}()

	select {
	case <-notify.ch:
	case <-time.After(5 * time.Second):
		t.Fatal("store did not start")
	}

	if err := store.Store("key", []byte("value")); err != nil {
		t.Fatal(err)
	}

	cancel()

	if err := <-done; err != nil {
		t.Fatal(err)
	}

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()

	value, ok, err := reopened.Load("key")
	if err != nil {
		t.Fatal(err)
	}

	if !ok || !bytes.Equal(value, []byte("value")) {
		t.Fatalf("Load(key) = %q, ok %v", value, ok)
	}
}

func validateTree(t *testing.T, store *Bytes) {
	t.Helper()

	root := store.root.Load()
	if root == 0 {
		return
	}

	var walk func(id uint64) (string, []uint64, []string)

	walk = func(id uint64) (string, []uint64, []string) {
		p, err := store.loadPage(id)
		if err != nil {
			t.Fatal(err)
		}

		header, err := parsePage(p.data)
		if err != nil {
			t.Fatal(err)
		}

		switch header.typ {
		case pageLeaf:
			entries, err := decodeLeafEntries(p.data[pageHeaderSize:], header.count)
			if err != nil {
				t.Fatal(err)
			}

			keys := make([]string, 0, len(entries))
			for i, entry := range entries {
				if i > 0 && entries[i-1].key >= entry.key {
					t.Fatalf("leaf %d keys are not sorted: %q then %q", id, entries[i-1].key, entry.key)
				}

				keys = append(keys, entry.key)
			}

			first := ""
			if len(entries) > 0 {
				first = entries[0].key
			}

			return first, []uint64{id}, keys
		case pageInternal:
			entries, err := decodeInternalEntries(p.data[pageHeaderSize:], header.count)
			if err != nil {
				t.Fatal(err)
			}

			ids := []uint64{}
			keys := []string{}
			first := ""

			for i, entry := range entries {
				if i > 0 && entries[i-1].key >= entry.key {
					t.Fatalf("internal %d keys are not sorted: %q then %q", id, entries[i-1].key, entry.key)
				}

				childFirst, childIDs, childKeys := walk(entry.child)
				if len(childKeys) > 0 && childFirst != entry.key {
					t.Fatalf("internal %d separator %q, child %d first key %q", id, entry.key, entry.child, childFirst)
				}

				if i == 0 {
					first = childFirst
				}

				ids = append(ids, childIDs...)
				keys = append(keys, childKeys...)
			}

			return first, ids, keys
		case pageFree, pageOverflow:
			t.Fatalf("page %d has type %d", id, header.typ)
		default:
			t.Fatalf("page %d has type %d", id, header.typ)
		}

		return "", nil, nil
	}

	_, _, keys := walk(root)

	for i := 1; i < len(keys); i++ {
		if keys[i-1] >= keys[i] {
			t.Fatalf("tree keys are not sorted: %q then %q", keys[i-1], keys[i])
		}
	}
}

func checkContents(t *testing.T, store *Bytes, expected map[string][]byte) {
	t.Helper()

	keys := make([]string, 0, len(expected))
	for key := range expected {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	got := []string{}

	for key, value := range store.All() {
		got = append(got, key)

		if !bytes.Equal(value, expected[key]) {
			t.Fatalf("All() value for %q = %q, want %q", key, value, expected[key])
		}
	}

	if fmt.Sprint(got) != fmt.Sprint(keys) {
		t.Fatalf("All() keys = %v, want %v", got, keys)
	}

	if len(expected) == 0 {
		return
	}

	from := keys[len(keys)/2]
	remaining := keys[len(keys)/2:]

	got = got[:0]
	for key := range store.From(from) {
		got = append(got, key)
	}

	if fmt.Sprint(got) != fmt.Sprint(remaining) {
		t.Fatalf("From(%q) = %v, want %v", from, got, remaining)
	}
}
