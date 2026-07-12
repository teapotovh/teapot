package s3cache

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

// writeFileAtomic writes to a temporary file and atomically swaps it via
// a rename with the desired file path. This prevents corrupted writes.
func writeFileAtomic(path string, data []byte) error {
	dir, pattern := filepath.Split(path)

	file, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return fmt.Errorf("error while creating temporary file: %w", err)
	}

	if _, err := io.Copy(file, bytes.NewReader(data)); err != nil {
		return fmt.Errorf("error while writing data to temporary file: %w", err)
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf("error while closing temporary file after write: %w", err)
	}

	return os.Rename(file.Name(), path)
}

// diskCapacity returns the total size, in bytes, of the filesystem that
// path resides on.
func diskCapacity(path string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}

	return stat.Blocks * uint64(stat.Bsize), nil
}
