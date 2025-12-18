package webhandler

import (
	"fmt"
	"net/http"

	"github.com/kataras/requestid"
	g "maragu.dev/gomponents"
	hxhttp "maragu.dev/gomponents-htmx/http"
	h "maragu.dev/gomponents/html"

	"github.com/teapotovh/teapot/lib/ui"
	"github.com/teapotovh/teapot/lib/ui/components"
)

// TODO: request id

type ErrorHandler[T error] func(w http.ResponseWriter, r *http.Request, err T, contact string) (ui.Component, error)

type ErrorHandlers struct {
	InternalHandler   ErrorHandler[InternalError]
	RedirectHandler   ErrorHandler[RedirectError]
	NotFoundHandler   ErrorHandler[error]
	BadRequestHandler ErrorHandler[error]
	GenericHandler    ErrorHandler[error]
}

func DefaultInternalHandler(
	w http.ResponseWriter,
	r *http.Request,
	err InternalError,
	contact string,
) (ui.Component, error) {
	w.WriteHeader(http.StatusInternalServerError)

	return ui.ComponentFunc(func(ctx ui.Context) g.Node {
		return components.ErrorDialog(
			ctx,
			fmt.Errorf("%w. please report this to: %s. request id: %s", err.External(), contact, requestid.Get(r)),
		)
	}), nil
}

// Ensure DefaultInternalHandler implements ErrorHandler[InternalError].
var _ ErrorHandler[InternalError] = DefaultInternalHandler

func DefaultRedirectHandler(
	w http.ResponseWriter,
	r *http.Request,
	err RedirectError,
	contact string,
) (ui.Component, error) {
	if hxhttp.IsRequest(r.Header) {
		hxhttp.SetLocation(w.Header(), err.URL())
	} else {
		http.Redirect(w, r, err.URL(), err.StatusCode())
	}

	return nil, nil //nolint:nilnil // this is expected
}

// Ensure DefaultRedirectHandler implements ErrorHandler[RedirectError].
var _ ErrorHandler[RedirectError] = DefaultRedirectHandler

func DefaultNotFoundHandler(
	w http.ResponseWriter,
	r *http.Request,
	err error,
	contact string,
) (ui.Component, error) {
	w.WriteHeader(http.StatusNotFound)

	return ui.ComponentFunc(func(ctx ui.Context) g.Node {
		return h.Main(h.H2(g.Text("404 - Not Found")))
	}), nil
}

// Ensure DefaultNotFundHandler implements ErrorHandler[error].
var _ ErrorHandler[error] = DefaultNotFoundHandler

func DefaultBadRequestHandler(
	w http.ResponseWriter,
	r *http.Request,
	err error,
	contact string,
) (ui.Component, error) {
	w.WriteHeader(http.StatusBadRequest)

	return ui.ComponentFunc(func(ctx ui.Context) g.Node {
		return components.ErrorDialog(
			ctx,
			fmt.Errorf("%w. please report this to: %s. request id: %s", err, contact, requestid.Get(r)),
		)
	}), nil
}

// Ensure DefaultBadRequestHandler implements ErrorHandler[error].
var _ ErrorHandler[error] = DefaultBadRequestHandler

func DefaultGenericHandler(
	w http.ResponseWriter,
	r *http.Request,
	err error,
	contact string,
) (ui.Component, error) {
	w.WriteHeader(http.StatusBadRequest)

	return ui.ComponentFunc(func(ctx ui.Context) g.Node {
		return components.ErrorDialog(
			ctx,
			fmt.Errorf("%w. please report this to: %s. request id: %s", err, contact, requestid.Get(r)),
		)
	}), nil
}

// Ensure DefaultGenericHandler implements ErrorHandler[error].
var _ ErrorHandler[error] = DefaultGenericHandler

var DefaultErrorHandlers = ErrorHandlers{
	InternalHandler: DefaultInternalHandler,
	RedirectHandler: DefaultRedirectHandler,
	NotFoundHandler: DefaultNotFoundHandler,
	GenericHandler:  DefaultGenericHandler,
}
