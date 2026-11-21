package files

import (
	"errors"
	"testing"
)

func TestCleanPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path string
		want string
	}{
		{"/", "/"},
		{"/a", "/a"},
		{"//a//b", "/a/b"},
		{"/a/./b", "/a/b"},
		{"/a/", "/a"},
		{"/a/b/", "/a/b"},
	}

	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			t.Parallel()

			got, err := CleanPath(test.path)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			if got != test.want {
				t.Fatalf("expected %q, got %q", test.want, got)
			}
		})
	}
}

func TestCleanPathInvalid(t *testing.T) {
	t.Parallel()

	paths := []string{
		"",
		"a",
		"/..",
		"/a/../b",
		"/a\x00b",
	}

	for _, p := range paths {
		if _, err := CleanPath(p); !errors.Is(err, ErrInvalidPath) {
			t.Fatalf("expected %v for %q, got %v", ErrInvalidPath, p, err)
		}
	}
}
