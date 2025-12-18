package httphandler

import "github.com/teapotovh/teapot/lib/httphandler"

type InternalError = httphandler.InternalError
type RedirectError = httphandler.RedirectError

var (
	ErrNotFound = httphandler.ErrNotFound
	ErrInternal = httphandler.ErrInternal
)
