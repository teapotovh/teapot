package files

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		valid bool
	}{
		{"file.txt", true},
		{"a", true},
		{"Caf\u00e9", true},
		{"emoji \U0001F600", true},
		{strings.Repeat("a", MaxNameBytes), true},
		{strings.Repeat("a", MaxNameBytes+1), false},
		{"", false},
		{".", false},
		{"..", false},
		{"a/b", false},
		{"a\x00b", false},
		{"\x01", false},
		{"\u007f", false},
		{"\u0085", false},
		{"\u200b", false},
		{"\u00ad", false},
		{"\uFDD0", false},
		{"\uFFFE", false},
		{"\U0001FFFE", false},
		{"e\u0301", false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateName(test.name)
			if test.valid && err != nil {
				t.Fatalf("expected valid name, got error: %v", err)
			}

			if !test.valid && !errors.Is(err, ErrInvalidName) {
				t.Fatalf("expected %v, got %v", ErrInvalidName, err)
			}
		})
	}
}

func TestValidateNameInvalidUTF8(t *testing.T) {
	t.Parallel()

	names := []string{
		string([]byte{0xff}),
		string([]byte{0xed, 0xa0, 0x80}),
	}

	for _, name := range names {
		if err := ValidateName(name); !errors.Is(err, ErrInvalidName) {
			t.Fatalf("expected %v for %q, got %v", ErrInvalidName, name, err)
		}
	}
}

func TestNormalizeName(t *testing.T) {
	t.Parallel()

	got, err := NormalizeName("e\u0301")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if want := "\u00e9"; got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}

	if _, err := NormalizeName("a/b"); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("expected %v, got %v", ErrInvalidName, err)
	}
}
