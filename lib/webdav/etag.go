package webdav

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// ErrInvalidEntityTag reports a malformed entity tag.
var ErrInvalidEntityTag = errors.New("webdav: invalid entity tag")

// ConditionalMatch represents the value of a conditional header
// according to RFC 9110 section 13.1. The value is either a wildcard
// or a comma-separated list of entity tags.
type ConditionalMatch string

// IfMatch returns the If-Match header values as a conditional match.
func IfMatch(header http.Header) ConditionalMatch {
	return ConditionalMatch(strings.Join(header.Values("If-Match"), ","))
}

// IfNoneMatch returns the If-None-Match header values as a conditional match.
func IfNoneMatch(header http.Header) ConditionalMatch {
	return ConditionalMatch(strings.Join(header.Values("If-None-Match"), ","))
}

func (val ConditionalMatch) IsSet() bool {
	return val != ""
}

func (val ConditionalMatch) IsWildcard() bool {
	return strings.TrimSpace(string(val)) == "*"
}

// MatchETag reports if the value matches the ETag with strong comparison.
func (val ConditionalMatch) MatchETag(etag string) (bool, error) {
	return val.match(etag, true)
}

// MatchETagWeak reports if the value matches the ETag with weak comparison.
func (val ConditionalMatch) MatchETagWeak(etag string) (bool, error) {
	return val.match(etag, false)
}

func (val ConditionalMatch) match(etag string, strong bool) (bool, error) {
	if !val.IsSet() {
		return false, nil
	}

	if val.IsWildcard() {
		return true, nil
	}

	tags, err := parseEntityTags(string(val))
	if err != nil {
		return false, err
	}

	for _, tag := range tags {
		if strong && tag.weak {
			continue
		}

		if tag.value == etag {
			return true, nil
		}
	}

	return false, nil
}

type entityTag struct {
	value string
	weak  bool
}

func parseEntityTags(value string) ([]entityTag, error) {
	var tags []entityTag

	for value != "" {
		value = strings.TrimLeft(value, " \t")
		if value == "" {
			break
		}

		if value[0] == ',' {
			value = value[1:]
			continue
		}

		tag, rest, err := parseEntityTag(value)
		if err != nil {
			return nil, err
		}

		tags = append(tags, tag)

		rest = strings.TrimLeft(rest, " \t")
		if rest != "" && rest[0] != ',' {
			return nil, fmt.Errorf("%w list %q", ErrInvalidEntityTag, value)
		}

		value = rest
	}

	return tags, nil
}

func parseEntityTag(value string) (tag entityTag, rest string, err error) {
	weak := strings.HasPrefix(value, "W/")
	if weak {
		value = value[2:]
	}

	if value == "" || value[0] != '"' {
		return entityTag{}, "", fmt.Errorf("%w %q", ErrInvalidEntityTag, value)
	}

	end := strings.IndexByte(value[1:], '"')
	if end < 0 {
		return entityTag{}, "", fmt.Errorf("%w %q", ErrInvalidEntityTag, value)
	}

	end++

	return entityTag{value: value[1:end], weak: weak}, value[end+1:], nil
}
