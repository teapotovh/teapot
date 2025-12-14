package kontakte

import (
	"errors"
	"fmt"
	"net/http"

	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

var ErrNotFound = errors.New("not found")

type statusError struct {
	error

	status int
}

func (e statusError) StatusCode() int {
	return e.status
}

func (e statusError) Unwrap() error {
	return e.error
}

func ErrorWithStatus(err error, status int) error {
	return statusError{error: err, status: status}
}

func ErrorDialog(err error) g.Node {
	return h.Div(h.Class("error-dialog"), g.Textf("Error: %s", err))
}

type errorWithStatusCode interface {
	StatusCode() int
}

func ErrorPage(r *http.Request, err error) g.Node {
	status := http.StatusInternalServerError
	if e, ok := err.(errorWithStatusCode); ok {
		status = e.StatusCode()
	}

	body := h.Section(h.Class("error"),
		h.H2(g.Textf("Error: %d %s", status, http.StatusText(status))),
		h.H3(g.Text(err.Error())),
	)
	return Page(r, "Error", body)
}

func NotFound(r *http.Request) (g.Node, error) {
	err := fmt.Errorf("could not %s %s", r.Method, r.URL)
	err = ErrorWithStatus(err, http.StatusNotFound)
	return ErrorPage(r, err), ErrorWithStatus(errors.Join(err, ErrNotFound), http.StatusNotFound)
}

func Unauthorized(r *http.Request) (g.Node, error) {
	err := fmt.Errorf("could not %s %s", r.Method, r.URL)
	err = ErrorWithStatus(err, http.StatusUnauthorized)
	return ErrorPage(r, err), err
}

func SuccessDialog(node g.Node) g.Node {
	return h.Div(h.Class("success-dialog"), node)
}
