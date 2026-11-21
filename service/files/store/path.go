package store

import (
	"path"
	"slices"
	"strings"
)

// PathSeparator is the separator used in all paths.
const PathSeparator = '/'

// CleanPath checks that p is an absolute path and returns its clean form.
func CleanPath(p string) (string, error) {
	if p == "" || p[0] != PathSeparator || strings.IndexByte(p, 0) >= 0 {
		return "", ErrInvalidPath
	}

	if slices.Contains(strings.Split(p, string(PathSeparator)), "..") {
		return "", ErrInvalidPath
	}

	return path.Clean(p), nil
}
