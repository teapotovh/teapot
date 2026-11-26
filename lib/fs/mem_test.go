package fs

import (
	"errors"
	"io"
	"os"
	"testing"
)

func TestMemoryFileReadWrite(t *testing.T) {
	t.Parallel()

	file := NewMemoryFile()

	if _, err := file.WriteAt([]byte("hello"), 2); err != nil {
		t.Fatal(err)
	}

	got := make([]byte, 7)
	if _, err := file.ReadAt(got, 0); err != nil {
		t.Fatal(err)
	}

	if want := "\x00\x00hello"; string(got) != want {
		t.Fatalf("ReadAt() = %q, want %q", got, want)
	}

	if _, err := file.ReadAt(make([]byte, 1), 7); !errors.Is(err, io.EOF) {
		t.Fatalf("ReadAt() past end error = %v, want %v", err, io.EOF)
	}
}

func TestMemoryFileTruncate(t *testing.T) {
	t.Parallel()

	file := NewMemoryFile()

	if _, err := file.WriteAt([]byte("hello"), 0); err != nil {
		t.Fatal(err)
	}

	if err := file.Truncate(3); err != nil {
		t.Fatal(err)
	}

	info, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}

	if info.Size() != 3 {
		t.Fatalf("Size() after shrink = %d, want 3", info.Size())
	}

	if err := file.Truncate(5); err != nil {
		t.Fatal(err)
	}

	info, err = file.Stat()
	if err != nil {
		t.Fatal(err)
	}

	if info.Size() != 5 {
		t.Fatalf("Size() after grow = %d, want 5", info.Size())
	}

	if err := file.Truncate(-1); !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("Truncate(-1) error = %v, want %v", err, os.ErrInvalid)
	}
}

func TestMemoryFSFiles(t *testing.T) {
	t.Parallel()

	files := NewMemoryFS()

	first, err := files.OpenFile("first", 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	again, err := files.OpenFile("first", 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	other, err := files.OpenFile("other", 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	if first != again {
		t.Fatal("OpenFile() returned a different file for the same name")
	}

	if first == other {
		t.Fatal("OpenFile() returned the same file for different names")
	}
}
