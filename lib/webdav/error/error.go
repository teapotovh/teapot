package error //nolint:predeclared

import (
	"errors"
	"fmt"
	"net/http"
)

type HTTPError struct {
	Code int
	Err  error
}

func HTTPErrorFromError(err error) *HTTPError {
	if err == nil {
		return nil
	}

	httpErr := &HTTPError{}
	if errors.As(err, &httpErr) {
		return httpErr
	} else {
		return &HTTPError{http.StatusInternalServerError, err}
	}
}

func IsNotFound(err error) bool {
	if httpErr, ok := errors.AsType[*HTTPError](err); ok {
		return httpErr.Code == http.StatusNotFound
	}

	return false
}

func HTTPErrorf(code int, format string, a ...any) *HTTPError {
	return &HTTPError{code, fmt.Errorf(format, a...)} //nolint:err113
}

func (err *HTTPError) Error() string {
	s := fmt.Sprintf("%v %v", err.Code, http.StatusText(err.Code))
	if err.Err != nil {
		return fmt.Sprintf("%v: %v", s, err.Err)
	} else {
		return s
	}
}

func (err *HTTPError) Unwrap() error {
	return err.Err
}
