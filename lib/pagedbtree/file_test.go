package pagedbtree

import (
	"errors"
	"testing"
)

func TestHeaderRoundTrip(t *testing.T) {
	t.Parallel()

	want := fileHeader{
		Magic:     fileMagic,
		Version:   fileVersion,
		PageSize:  pageSize,
		PageCount: 42,
		Root:      17,
		FreeHead:  9,
	}

	got, err := decodeHeader(encodeHeader(want))
	if err != nil {
		t.Fatal(err)
	}

	if got != want {
		t.Fatalf("decodeHeader(encodeHeader(header)) = %#v, want %#v", got, want)
	}
}

func TestHeaderChecksum(t *testing.T) {
	t.Parallel()

	data := encodeHeader(fileHeader{
		Magic:     fileMagic,
		Version:   fileVersion,
		PageSize:  pageSize,
		PageCount: 1,
		Root:      1,
	})

	data[headerSize-1] ^= 0xff

	if _, err := decodeHeader(data); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("decodeHeader() error = %v, want %v", err, ErrCorrupt)
	}
}

func TestHeaderTruncated(t *testing.T) {
	t.Parallel()

	if _, err := decodeHeader(make([]byte, headerSize-1)); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("decodeHeader() error = %v, want %v", err, ErrCorrupt)
	}
}
