package httphandler

import "errors"

var (
	ErrNotFound   = errors.New("not found")
	ErrInternal   = errors.New("internal error")
	ErrBadRequest = errors.New("bad request")
)
