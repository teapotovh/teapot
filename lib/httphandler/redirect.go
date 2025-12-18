package httphandler

import "fmt"

type RedirectError struct {
	url    string
	status int
}

func NewRedirectError(url string, status int) RedirectError {
	return RedirectError{url: url, status: status}
}

func (re RedirectError) URL() string     { return re.url }
func (re RedirectError) StatusCode() int { return re.status }

// Ensure RedirectError implements error.
var _ error = RedirectError{}

func (re RedirectError) Error() string {
	return fmt.Sprintf("redirect status=%d, to=%s", re.status, re.url)
}
