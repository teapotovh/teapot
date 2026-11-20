package pagedbtree

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"os"
)

// File is the file interface used by the store.
type File interface {
	io.ReaderAt
	io.WriterAt
	Truncate(size int64) error
	Stat() (os.FileInfo, error)
	Close() error
}

// FS opens files for the store.
type FS interface {
	OpenFile(name string, flag int, perm os.FileMode) (File, error)
}

const (
	fileMagic   = 0x50425442
	fileVersion = 1
	headerSize  = 64

	headerCRCOffset = 12
)

type fileHeader struct {
	Magic     uint32
	Version   uint32
	PageSize  uint32
	PageCount uint64
	Root      uint64
	FreeHead  uint64
}

func encodeHeader(header fileHeader) []byte {
	data := make([]byte, headerSize)
	binary.LittleEndian.PutUint32(data[0:], header.Magic)
	binary.LittleEndian.PutUint32(data[4:], header.Version)
	binary.LittleEndian.PutUint32(data[8:], header.PageSize)
	binary.LittleEndian.PutUint64(data[16:], header.PageCount)
	binary.LittleEndian.PutUint64(data[24:], header.Root)
	binary.LittleEndian.PutUint64(data[32:], header.FreeHead)
	binary.LittleEndian.PutUint32(data[headerCRCOffset:], crc32.ChecksumIEEE(data))

	return data
}

func decodeHeader(data []byte) (fileHeader, error) {
	if len(data) < headerSize {
		return fileHeader{}, fmt.Errorf("truncated file header: %w", ErrCorrupt)
	}

	copyData := append([]byte(nil), data[:headerSize]...)
	want := binary.LittleEndian.Uint32(copyData[headerCRCOffset:])
	binary.LittleEndian.PutUint32(copyData[headerCRCOffset:], 0)

	if got := crc32.ChecksumIEEE(copyData); got != want {
		return fileHeader{}, fmt.Errorf("file header checksum mismatch: %w", ErrCorrupt)
	}

	return fileHeader{
		Magic:     binary.LittleEndian.Uint32(copyData[0:]),
		Version:   binary.LittleEndian.Uint32(copyData[4:]),
		PageSize:  binary.LittleEndian.Uint32(copyData[8:]),
		PageCount: binary.LittleEndian.Uint64(copyData[16:]),
		Root:      binary.LittleEndian.Uint64(copyData[24:]),
		FreeHead:  binary.LittleEndian.Uint64(copyData[32:]),
	}, nil
}
