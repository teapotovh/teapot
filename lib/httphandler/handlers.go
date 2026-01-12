package httphandler

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/kataras/requestid"
)

var (
	ErrLengthMisatch = errors.New("length mismatch")
)

type ErrorHandler[T error] func(w http.ResponseWriter, r *http.Request, err T, contact string) error

type ErrorHandlers struct {
	InternalHandler   ErrorHandler[InternalError]
	RedirectHandler   ErrorHandler[RedirectError]
	NotFoundHandler   ErrorHandler[error]
	BadRequestHandler ErrorHandler[error]
	GenericHandler    ErrorHandler[error]
}

func DefaultInternalHandler(w http.ResponseWriter, r *http.Request, err InternalError, contact string) error {
	msg := fmt.Sprintf(
		"%s. please report this to: %s. request id: %s",
		err.External().Error(),
		contact,
		requestid.Get(r),
	)

	return Write(w, http.StatusInternalServerError, []byte(msg))
}

// Ensure DefaultInternalHandler implements ErrorHandler[InternalError].
var _ ErrorHandler[InternalError] = DefaultInternalHandler

func DefaultRedirectHandler(w http.ResponseWriter, r *http.Request, err RedirectError, contact string) error {
	http.Redirect(w, r, err.url, err.status)
	return nil
}

// Ensure DefaultRedirectHandler implements ErrorHandler[RedirectError].
var _ ErrorHandler[RedirectError] = DefaultRedirectHandler

func DefaultNotFoundHandler(w http.ResponseWriter, r *http.Request, err error, contact string) error {
	return Write(w, http.StatusNotFound, []byte(err.Error()))
}

// Ensure DefaultNotFoundHandler implements ErrorHandler[error].
var _ ErrorHandler[error] = DefaultNotFoundHandler

func DefaultBadRequestHandler(w http.ResponseWriter, r *http.Request, err error, contact string) error {
	return Write(w, http.StatusBadRequest, []byte(err.Error()))
}

// Ensure DefaultBadRequestHandler implements ErrorHandler[error].
var _ ErrorHandler[error] = DefaultBadRequestHandler

func DefaultGenericHandler(w http.ResponseWriter, r *http.Request, err error, contact string) error {
	msg := "unhandled error. please report this to: " + contact + ". request id: " + requestid.Get(r)
	return Write(w, http.StatusBadRequest, []byte(msg))
}

// Ensure DefaultGenericHandler implements ErrorHandler[error].
var _ ErrorHandler[error] = DefaultGenericHandler

// Write is a helper to have proper error wrapping and HTTP status code writing
// when finishing off an HTTP response. It shall be used at the end of HTTP
// handler functions.
func Write(w http.ResponseWriter, status int, data []byte) error {
	w.WriteHeader(status)

	l, err := w.Write(data)
	if err != nil {
		return fmt.Errorf("error while writing response data: %w", err)
	}

	if l != len(data) {
		return fmt.Errorf("expected to write %d bytes, instead wrote %d: %w", len(data), l, ErrLengthMisatch)
	}

	return nil
}

var DefaultErrorHandlers = ErrorHandlers{
	InternalHandler:   DefaultInternalHandler,
	RedirectHandler:   DefaultRedirectHandler,
	NotFoundHandler:   DefaultNotFoundHandler,
	BadRequestHandler: DefaultBadRequestHandler,
	GenericHandler:    DefaultGenericHandler,
}
