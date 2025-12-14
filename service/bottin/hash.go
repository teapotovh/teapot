package bottin

import (
	"crypto/md5" //nolint:gosec
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/jsimonetti/pwscheme/ssha256"
	"github.com/jsimonetti/pwscheme/ssha512"
)

type HashType string

var ErrNoValidHash = errors.New("no valid hash found")

const (
	MD5     HashType = "{MD5}"
	SSHA256 HashType = "{SSHA256}"
	SSHA512 HashType = "{SSHA512}"
)

func ssha512Encode(passwd string) (string, error) {
	hash, err := ssha512.Generate(passwd, 16)
	if err != nil {
		return "", fmt.Errorf("error while hashing password with ssha512: %w", err)
	}

	return hash, nil
}

// Matches matches the encoded password and the raw password
func matchPassword(schemaHash string, passwd string) (bool, error) {
	hashType, hash, err := parseHash(schemaHash)
	if err != nil {
		return false, fmt.Errorf("invalid password hash stored: %w", err)
	}

	switch hashType {
	// This is required for backwards compatibility with slapd-generated passwords
	case MD5:
		bytes := md5.Sum([]byte(passwd)) //nolint:gosec
		based := base64.StdEncoding.EncodeToString(bytes[:])
		return based == hash, nil
	case SSHA256:
		return ssha256.Validate(passwd, string(SSHA256)+hash)
	case SSHA512:
		return ssha512.Validate(passwd, string(SSHA512)+hash)
	}

	return false, errors.New("no matching hash type found")
}

func parseHash(hash string) (HashType, string, error) {
	if strings.HasPrefix(hash, string(MD5)) {
		return MD5, hash[len(MD5):], nil
	} else if strings.HasPrefix(hash, string(SSHA256)) {
		return SSHA256, hash[len(SSHA256):], nil
	} else if strings.HasPrefix(hash, string(SSHA512)) {
		return SSHA512, hash[len(SSHA512):], nil
	} else {
		return HashType(""), "", ErrNoValidHash
	}
}
