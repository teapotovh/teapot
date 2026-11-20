package pagedbtree

import (
	"bytes"
	"errors"
	"testing"
)

func checkLeafEntries(t *testing.T, got, want []leafEntry) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("decoded %d entries, want %d", len(got), len(want))
	}

	for i := range want {
		if got[i].key != want[i].key {
			t.Fatalf("entry %d key = %q, want %q", i, got[i].key, want[i].key)
		}

		if got[i].size != want[i].size {
			t.Fatalf("entry %d size = %d, want %d", i, got[i].size, want[i].size)
		}

		if got[i].overflow != want[i].overflow {
			t.Fatalf("entry %d overflow = %d, want %d", i, got[i].overflow, want[i].overflow)
		}

		if !bytes.Equal(got[i].value, want[i].value) {
			t.Fatalf("entry %d value = %q, want %q", i, got[i].value, want[i].value)
		}
	}
}

func TestLeafPageRoundTrip(t *testing.T) {
	t.Parallel()

	entries := []leafEntry{
		{key: "alpha", value: []byte("one"), size: 3},
		{key: "beta"},
		{key: "gamma", size: 5000, overflow: 7},
	}

	payload := encodeLeafEntries(entries)

	p, err := makePage(1, pageLeaf, payload, 0, toUint16(len(entries)))
	if err != nil {
		t.Fatal(err)
	}

	header, err := parsePage(p.data)
	if err != nil {
		t.Fatal(err)
	}

	if header.typ != pageLeaf || header.count != toUint16(len(entries)) {
		t.Fatalf("header = %#v, want leaf with %d entries", header, len(entries))
	}

	got, err := decodeLeafEntries(p.data[pageHeaderSize:], header.count)
	if err != nil {
		t.Fatal(err)
	}

	checkLeafEntries(t, got, entries)
}

func TestInternalPageRoundTrip(t *testing.T) {
	t.Parallel()

	entries := []internalEntry{
		{child: 1, key: "alpha"},
		{child: 2, key: "beta"},
		{child: 3, key: "gamma"},
	}

	payload := encodeInternalEntries(entries)

	p, err := makePage(1, pageInternal, payload, 0, toUint16(len(entries)))
	if err != nil {
		t.Fatal(err)
	}

	header, err := parsePage(p.data)
	if err != nil {
		t.Fatal(err)
	}

	if header.typ != pageInternal || header.count != toUint16(len(entries)) {
		t.Fatalf("header = %#v, want internal with %d entries", header, len(entries))
	}

	got, err := decodeInternalEntries(p.data[pageHeaderSize:], header.count)
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != len(entries) {
		t.Fatalf("decoded %d entries, want %d", len(got), len(entries))
	}

	for i := range entries {
		if got[i] != entries[i] {
			t.Fatalf("entry %d = %#v, want %#v", i, got[i], entries[i])
		}
	}
}

func TestPageChecksum(t *testing.T) {
	t.Parallel()

	p, err := makePage(1, pageLeaf, encodeLeafEntries([]leafEntry{{key: "key"}}), 0, 1)
	if err != nil {
		t.Fatal(err)
	}

	p.data[pageHeaderSize] ^= 0xff

	if _, err := parsePage(p.data); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("parsePage() error = %v, want %v", err, ErrCorrupt)
	}
}

func TestPageInvalidType(t *testing.T) {
	t.Parallel()

	p, err := makePage(1, pageLeaf, nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	p.data[pageTypeOffset] = 0xff

	if _, err := parsePage(p.data); !errors.Is(err, ErrInvalidPage) {
		t.Fatalf("parsePage() error = %v, want %v", err, ErrInvalidPage)
	}
}

func TestDecodeTruncatedLeaf(t *testing.T) {
	t.Parallel()

	if _, err := decodeLeafEntries(make([]byte, leafEntryHeaderSize-1), 1); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("decodeLeafEntries() error = %v, want %v", err, ErrCorrupt)
	}
}
