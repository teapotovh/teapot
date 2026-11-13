package internal

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testDisplayName = "home"

var errTestFailure = errors.New("boom")

func TestDepthConstants(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Depth(0), DepthZero)
	assert.Equal(t, Depth(1), DepthOne)
	assert.Equal(t, Depth(-1), DepthInfinity)

	assert.Equal(t, "0", DepthZero.String())
	assert.Equal(t, "1", DepthOne.String())
	assert.Equal(t, "infinity", DepthInfinity.String())

	assert.Panics(t, func() { _ = Depth(42).String() })
}

func TestParseDepth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		want    Depth
		wantErr error
	}{
		{name: "zero", value: "0", want: DepthZero},
		{name: "one", value: "1", want: DepthOne},
		{name: "infinity", value: "infinity", want: DepthInfinity},
		{name: "empty", value: "", want: 0, wantErr: ErrInvalidDepthValue},
		{name: "two", value: "2", want: 0, wantErr: ErrInvalidDepthValue},
		{name: "negative", value: "-1", want: 0, wantErr: ErrInvalidDepthValue},
		{name: "capitalized", value: "Infinity", want: 0, wantErr: ErrInvalidDepthValue},
		{name: "leading space", value: " 1", want: 0, wantErr: ErrInvalidDepthValue},
		{name: "trailing space", value: "infinity ", want: 0, wantErr: ErrInvalidDepthValue},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseDepth(tt.value)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Equal(t, tt.want, got)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseOverwrite(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		want    bool
		wantErr error
	}{
		{name: "true", value: "T", want: true},
		{name: "false", value: "F", want: false},
		{name: "empty", value: "", want: false, wantErr: ErrInvalidOverwriteValue},
		{name: "lowercase true", value: "t", want: false, wantErr: ErrInvalidOverwriteValue},
		{name: "lowercase false", value: "f", want: false, wantErr: ErrInvalidOverwriteValue},
		{name: "word", value: "true", want: false, wantErr: ErrInvalidOverwriteValue},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseOverwrite(tt.value)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Equal(t, tt.want, got)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFormatOverwrite(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "T", FormatOverwrite(true))
	assert.Equal(t, "F", FormatOverwrite(false))

	for _, value := range []string{"T", "F"} {
		parsed, err := ParseOverwrite(value)
		require.NoError(t, err)
		assert.Equal(t, value, FormatOverwrite(parsed))
	}
}
