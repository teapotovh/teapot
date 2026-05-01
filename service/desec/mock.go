package desec

import (
	"io"
	"log/slog"
	"net/http"
	"strings"
)

type MockTransport struct {
	logger *slog.Logger
}

// RoundTrip implements http.RoundTripper.
func (m *MockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	attrs := []any{
		slog.String("method", req.Method),
		slog.String("url", req.URL.String()),
	}

	if req.Body != nil {
		body, _ := io.ReadAll(req.Body)
		attrs = append(attrs, slog.String("body", string(body)))
	}

	m.logger.Info("desec client http request", attrs...)

	header := http.Header{
		"Content-Type": []string{"application/json"},
	}

	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Body:       io.NopCloser(strings.NewReader("[]")),
		Header:     header,
		Request:    req,
	}, nil
}
