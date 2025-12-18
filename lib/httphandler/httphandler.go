package httphandler

import (
	"errors"
	"log/slog"
	"net/http"
)

type HTTPHandlerFunc func(w http.ResponseWriter, r *http.Request) error

type HTTPHandlerConfig struct {
	Contact string
}

type HTTPHandler struct {
	logger *slog.Logger

	contact           string
	internalHandler   ErrorHandler[InternalError]
	redirectHandler   ErrorHandler[RedirectError]
	notFoundHandler   ErrorHandler[error]
	badRequestHandler ErrorHandler[error]
	genericHandler    ErrorHandler[error]
}

func NewHTTPHandler(config HTTPHandlerConfig, handlers ErrorHandlers, logger *slog.Logger) *HTTPHandler {
	return &HTTPHandler{
		logger: logger,

		contact:           config.Contact,
		internalHandler:   handlers.InternalHandler,
		redirectHandler:   handlers.RedirectHandler,
		notFoundHandler:   handlers.NotFoundHandler,
		badRequestHandler: handlers.BadRequestHandler,
		genericHandler:    handlers.GenericHandler,
	}
}

func (he *HTTPHandler) Adapt(fn HTTPHandlerFunc) http.Handler {
	adaptor := func(w http.ResponseWriter, r *http.Request) {
		err := fn(w, r)
		if err != nil {
			var (
				ierr InternalError
				rerr RedirectError
			)

			if errors.As(err, &ierr) {
				he.logger.ErrorContext(r.Context(), "error while handling request", "err", err)
				err = he.internalHandler(w, r, ierr, he.contact)
			} else if errors.As(err, &rerr) {
				err = he.redirectHandler(w, r, rerr, he.contact)
			} else if errors.Is(err, ErrNotFound) {
				err = he.notFoundHandler(w, r, err, he.contact)
			} else if errors.Is(err, ErrBadRequest) {
				err = he.badRequestHandler(w, r, err, he.contact)
			} else {
				he.logger.ErrorContext(r.Context(), "unhandled error while handling request", "err", err)
				err = he.genericHandler(w, r, err, he.contact)
			}
		}

		if err != nil {
			he.logger.Error("error when writing error response", "err", err)
		}
	}

	return http.HandlerFunc(adaptor)
}
