package pagedbtree

import (
	"container/list"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"sync/atomic"
)

var (
	// ErrCorrupt reports a page or header that failed validation.
	ErrCorrupt = errors.New("corrupt data")
	// ErrInvalidPage reports a page with an unknown type or a bad size.
	ErrInvalidPage = errors.New("invalid page")
	// ErrEntryTooLarge reports an entry that cannot be split.
	ErrEntryTooLarge = errors.New("entry too large")
)

const (
	pageSize        = 4 << 10
	pageHeaderSize  = 32
	pagePayloadSize = pageSize - pageHeaderSize

	pageTypeOffset  = 0
	pageCountOffset = 2
	pageCRCOffset   = 4
	pageNextOffset  = 8

	leafEntryHeaderSize     = 16
	internalEntryHeaderSize = 12
	maxKeyLength            = pagePayloadSize - leafEntryHeaderSize

	overflowFlag = 1 << 0

	maxInlineValue = 1024
)

type pageType uint8

const (
	pageFree pageType = iota
	pageLeaf
	pageInternal
	pageOverflow
)

type page struct {
	id    uint64
	data  []byte
	dirty atomic.Bool
	used  atomic.Bool
	elem  *list.Element
}

type pageHeader struct {
	typ   pageType
	count uint16
	next  uint64
}

type leafEntry struct {
	key      string
	value    []byte
	size     uint32
	overflow uint64
}

type internalEntry struct {
	child uint64
	key   string
}

func makePage(id uint64, typ pageType, payload []byte, next uint64, count uint16) (*page, error) {
	if id == 0 {
		return nil, ErrInvalidPage
	}

	if len(payload) > pagePayloadSize {
		return nil, ErrEntryTooLarge
	}

	data := make([]byte, pageSize)
	data[pageTypeOffset] = byte(typ)
	binary.LittleEndian.PutUint16(data[pageCountOffset:], count)
	binary.LittleEndian.PutUint64(data[pageNextOffset:], next)
	copy(data[pageHeaderSize:], payload)
	binary.LittleEndian.PutUint32(data[pageCRCOffset:], crc32.ChecksumIEEE(data[pageHeaderSize:]))

	p := &page{id: id, data: data}
	p.dirty.Store(true)
	p.used.Store(true)

	return p, nil
}

func parsePage(data []byte) (pageHeader, error) {
	if len(data) != pageSize {
		return pageHeader{}, ErrInvalidPage
	}

	header := pageHeader{
		typ:   pageType(data[pageTypeOffset]),
		count: binary.LittleEndian.Uint16(data[pageCountOffset:]),
		next:  binary.LittleEndian.Uint64(data[pageNextOffset:]),
	}

	switch header.typ {
	case pageFree, pageLeaf, pageInternal, pageOverflow:
	default:
		return pageHeader{}, fmt.Errorf("unknown page type %d: %w", header.typ, ErrInvalidPage)
	}

	want := binary.LittleEndian.Uint32(data[pageCRCOffset:])

	got := crc32.ChecksumIEEE(data[pageHeaderSize:])
	if want != got {
		return pageHeader{}, fmt.Errorf("page checksum mismatch: %w", ErrCorrupt)
	}

	return header, nil
}

func leafEntrySize(entry leafEntry) int {
	size := leafEntryHeaderSize + len(entry.key)
	if entry.overflow == 0 {
		size += len(entry.value)
	}

	return size
}

func leafEntriesSize(entries []leafEntry) int {
	size := 0
	for _, entry := range entries {
		size += leafEntrySize(entry)
	}

	return size
}

func encodeLeafEntries(entries []leafEntry) []byte {
	payload := make([]byte, leafEntriesSize(entries))
	offset := 0

	for _, entry := range entries {
		binary.LittleEndian.PutUint16(payload[offset:], toUint16(len(entry.key)))

		flags := uint16(0)
		if entry.overflow != 0 {
			flags = overflowFlag
		}

		binary.LittleEndian.PutUint16(payload[offset+2:], flags)
		binary.LittleEndian.PutUint32(payload[offset+4:], entry.size)
		binary.LittleEndian.PutUint64(payload[offset+8:], entry.overflow)
		offset += leafEntryHeaderSize
		offset += copy(payload[offset:], entry.key)

		if entry.overflow == 0 {
			offset += copy(payload[offset:], entry.value)
		}
	}

	return payload
}

func decodeLeafEntries(payload []byte, count uint16) ([]leafEntry, error) {
	entries := make([]leafEntry, 0, count)
	offset := 0

	for range count {
		if offset+leafEntryHeaderSize > len(payload) {
			return nil, fmt.Errorf("truncated leaf entry: %w", ErrCorrupt)
		}

		keyLength := int(binary.LittleEndian.Uint16(payload[offset:]))
		flags := binary.LittleEndian.Uint16(payload[offset+2:])
		size := binary.LittleEndian.Uint32(payload[offset+4:])
		overflow := binary.LittleEndian.Uint64(payload[offset+8:])
		offset += leafEntryHeaderSize

		if offset+keyLength > len(payload) {
			return nil, fmt.Errorf("truncated leaf key: %w", ErrCorrupt)
		}

		key := string(payload[offset : offset+keyLength])
		offset += keyLength

		entry := leafEntry{key: key, size: size, overflow: overflow}

		if flags&overflowFlag == 0 {
			if uint64(size) > toUint64(len(payload)-offset) {
				return nil, fmt.Errorf("truncated leaf value: %w", ErrCorrupt)
			}

			entry.value = payload[offset : offset+int(size)]
			offset += int(size)
		} else if overflow == 0 {
			return nil, fmt.Errorf("overflow entry without a page: %w", ErrCorrupt)
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

func encodeInternalEntries(entries []internalEntry) []byte {
	size := 0
	for _, entry := range entries {
		size += internalEntryHeaderSize + len(entry.key)
	}

	payload := make([]byte, size)
	offset := 0

	for _, entry := range entries {
		binary.LittleEndian.PutUint64(payload[offset:], entry.child)
		binary.LittleEndian.PutUint16(payload[offset+8:], toUint16(len(entry.key)))
		offset += internalEntryHeaderSize
		offset += copy(payload[offset:], entry.key)
	}

	return payload
}

func decodeInternalEntries(payload []byte, count uint16) ([]internalEntry, error) {
	entries := make([]internalEntry, 0, count)
	offset := 0

	for range count {
		if offset+internalEntryHeaderSize > len(payload) {
			return nil, fmt.Errorf("truncated internal entry: %w", ErrCorrupt)
		}

		child := binary.LittleEndian.Uint64(payload[offset:])
		keyLength := int(binary.LittleEndian.Uint16(payload[offset+8:]))
		offset += internalEntryHeaderSize

		if offset+keyLength > len(payload) {
			return nil, fmt.Errorf("truncated internal key: %w", ErrCorrupt)
		}

		entries = append(entries, internalEntry{
			child: child,
			key:   string(payload[offset : offset+keyLength]),
		})
		offset += keyLength
	}

	return entries, nil
}

func toUint16(value int) uint16 {
	return uint16(value) //nolint:gosec // the value fits in a page.
}

func toUint32(value int) uint32 {
	return uint32(value) //nolint:gosec // the value is checked before use.
}

func toUint64(value int) uint64 {
	return uint64(value) //nolint:gosec // the value is not negative.
}

func toInt(value uint64) int {
	return int(value) //nolint:gosec // the page count fits in an int.
}
