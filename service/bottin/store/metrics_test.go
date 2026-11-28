package store

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "success", err: nil, want: metricsStatusSuccess},
		{name: "not found", err: ErrNotFound, want: metricsStatusNotFound},
		{name: "wrapped not found", err: fmt.Errorf("lookup: %w", ErrNotFound), want: metricsStatusNotFound},
		{name: "error", err: assert.AnError, want: metricsStatusError},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, status(test.err))
		})
	}
}
