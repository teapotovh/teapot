package fs

import (
	"io"
	"os"
	"sync"
	"time"
)

const memName = "memory"

// MemoryFS is an in-memory file system.
type MemoryFS struct {
	mu    sync.Mutex
	files map[string]*MemoryFile
}

// NewMemoryFS creates an in-memory file system.
func NewMemoryFS() *MemoryFS {
	return &MemoryFS{files: map[string]*MemoryFile{}}
}

// File returns the file with the given name, and creates it if it does not exist.
func (f *MemoryFS) File(name string) *MemoryFile {
	f.mu.Lock()
	defer f.mu.Unlock()

	file, ok := f.files[name]
	if !ok {
		file = NewMemoryFile()
		f.files[name] = file
	}

	return file
}

// OpenFile implements FS.
func (f *MemoryFS) OpenFile(name string, _ int, _ os.FileMode) (File, error) {
	return f.File(name), nil
}

// MemoryFile is a writable in-memory file.
type MemoryFile struct {
	mu       sync.Mutex
	data     []byte
	writeErr error
}

// NewMemoryFile creates an in-memory file.
func NewMemoryFile() *MemoryFile {
	return &MemoryFile{}
}

// ReadAt implements File.
func (f *MemoryFile) ReadAt(p []byte, offset int64) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if offset < 0 {
		return 0, os.ErrInvalid
	}

	if offset >= int64(len(f.data)) {
		return 0, io.EOF
	}

	n := copy(p, f.data[offset:])
	if n < len(p) {
		return n, io.EOF
	}

	return n, nil
}

// WriteAt implements File.
func (f *MemoryFile) WriteAt(p []byte, offset int64) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.writeErr != nil {
		return 0, f.writeErr
	}

	end := offset + int64(len(p))
	if end > int64(len(f.data)) {
		data := make([]byte, end)
		copy(data, f.data)
		f.data = data
	}

	copy(f.data[offset:], p)

	return len(p), nil
}

// Truncate implements File.
func (f *MemoryFile) Truncate(size int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if size < 0 {
		return os.ErrInvalid
	}

	data := make([]byte, size)
	copy(data, f.data)
	f.data = data

	return nil
}

// Stat implements File.
func (f *MemoryFile) Stat() (os.FileInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return memoryFileInfo{size: int64(len(f.data))}, nil
}

// Close implements File.
func (f *MemoryFile) Close() error {
	return nil
}

// SetWriteError makes all later writes fail with err.
func (f *MemoryFile) SetWriteError(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.writeErr = err
}

// Corrupt flips all bits of the byte at offset.
func (f *MemoryFile) Corrupt(offset int) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.data[offset] ^= 0xff
}

// Bytes returns a copy of the file data.
func (f *MemoryFile) Bytes() []byte {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]byte(nil), f.data...)
}

type memoryFileInfo struct {
	size int64
}

func (i memoryFileInfo) Name() string       { return memName }
func (i memoryFileInfo) Size() int64        { return i.size }
func (i memoryFileInfo) Mode() os.FileMode  { return 0o600 }
func (i memoryFileInfo) ModTime() time.Time { return time.Time{} }
func (i memoryFileInfo) IsDir() bool        { return false }
func (i memoryFileInfo) Sys() any           { return nil }

// Ensure MemoryFS implements FS.
var _ FS = (*MemoryFS)(nil)

// Ensure MemoryFile implements File.
var _ File = (*MemoryFile)(nil)
