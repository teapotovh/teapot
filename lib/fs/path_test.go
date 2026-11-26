package fs

import (
	"errors"
	"testing"
)

func TestCleanPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		want string
		err  error
	}{
		{name: "root", path: "/", want: "/"},
		{name: "simple", path: "/a/b", want: "/a/b"},
		{name: "clean", path: "/a//b/./c/", want: "/a/b/c"},
		{name: "empty", path: "", err: ErrInvalidPath},
		{name: "relative", path: "a/b", err: ErrInvalidPath},
		{name: "parent", path: "/a/../b", err: ErrInvalidPath},
		{name: "nul", path: "/a\x00b", err: ErrInvalidPath},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := CleanPath(test.path)
			if !errors.Is(err, test.err) {
				t.Fatalf("CleanPath(%q) error = %v, want %v", test.path, err, test.err)
			}

			if got != test.want {
				t.Fatalf("CleanPath(%q) = %q, want %q", test.path, got, test.want)
			}
		})
	}
}
