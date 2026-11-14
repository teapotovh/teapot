package webdav

import (
	"errors"
	"net/http"

	daverr "github.com/teapotovh/teapot/lib/webdav/error"
)

// ErrPreconditionFailed reports that a request precondition is false.
var ErrPreconditionFailed = errors.New("webdav: precondition failed")

// Preconditions holds the validators of a conditional request.
type Preconditions struct {
	IfMatch     ConditionalMatch
	IfNoneMatch ConditionalMatch
}

// Outcome reports the result of a precondition evaluation.
type Outcome int

const (
	// Proceed means that the request can continue.
	Proceed Outcome = iota
	// NotModified means that If-None-Match matched the current representation.
	NotModified
	// PreconditionFailed means that If-Match did not match the current representation.
	PreconditionFailed
)

// PreconditionsFromHeader parses and validates conditional request headers.
func PreconditionsFromHeader(header http.Header) (Preconditions, error) {
	pre := Preconditions{
		IfMatch:     IfMatch(header),
		IfNoneMatch: IfNoneMatch(header),
	}

	if err := pre.IfMatch.Validate(); err != nil {
		return Preconditions{}, daverr.HTTPErrorf(http.StatusBadRequest, "webdav: invalid If-Match header: %v", err)
	}

	if err := pre.IfNoneMatch.Validate(); err != nil {
		return Preconditions{}, daverr.HTTPErrorf(
			http.StatusBadRequest,
			"webdav: invalid If-None-Match header: %v",
			err,
		)
	}

	return pre, nil
}

// Evaluate checks the preconditions against the current representation.
func (pre Preconditions) Evaluate(exists bool, etag string) (Outcome, error) {
	if pre.IfMatch.IsSet() {
		if !exists {
			return PreconditionFailed, nil
		}

		match, err := pre.IfMatch.MatchETag(etag)
		if err != nil {
			return Proceed, err
		}

		if !match {
			return PreconditionFailed, nil
		}
	}

	if pre.IfNoneMatch.IsSet() && exists {
		match, err := pre.IfNoneMatch.MatchETagWeak(etag)
		if err != nil {
			return Proceed, err
		}

		if match {
			return NotModified, nil
		}
	}

	return Proceed, nil
}

// CheckWrite returns ErrPreconditionFailed when a write must not continue.
func (pre Preconditions) CheckWrite(exists bool, etag string) error {
	outcome, err := pre.Evaluate(exists, etag)
	if err != nil {
		return err
	}

	if outcome != Proceed {
		return ErrPreconditionFailed
	}

	return nil
}
