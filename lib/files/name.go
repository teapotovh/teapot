package files

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// MaxNameBytes is the maximum size of a name in bytes.
const MaxNameBytes = 255

// NormalizeName returns the NFC form of name and checks it.
func NormalizeName(name string) (string, error) {
	if !utf8.ValidString(name) {
		return "", ErrInvalidName
	}

	name = norm.NFC.String(name)
	if err := ValidateName(name); err != nil {
		return "", err
	}

	return name, nil
}

// ValidateName reports whether name is a valid file name.
func ValidateName(name string) error {
	if len(name) == 0 || len(name) > MaxNameBytes {
		return ErrInvalidName
	}

	if !utf8.ValidString(name) {
		return ErrInvalidName
	}

	if name == "." || name == ".." || strings.ContainsRune(name, PathSeparator) {
		return ErrInvalidName
	}

	if !norm.NFC.IsNormalString(name) {
		return ErrInvalidName
	}

	for _, r := range name {
		if r == 0 || unicode.IsControl(r) || unicode.In(r, unicode.Cf) || isNonCharacter(r) {
			return ErrInvalidName
		}
	}

	return nil
}

func isNonCharacter(r rune) bool {
	return (r >= 0xFDD0 && r <= 0xFDEF) || r&0xFFFF == 0xFFFE || r&0xFFFF == 0xFFFF
}
