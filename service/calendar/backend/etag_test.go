package backend

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/teapotovh/teapot/lib/webdav"
)

var errTestMatcher = errors.New("test matcher error")

func TestNegateETagMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		inner   bool
		err     error
		want    bool
		wantErr bool
	}{
		{name: "inner matches", inner: true, err: nil, want: false, wantErr: false},
		{name: "inner does not match", inner: false, err: nil, want: true, wantErr: false},
		{name: "inner returns an error", inner: false, err: errTestMatcher, want: true, wantErr: true},
		{name: "inner matches with an error", inner: true, err: errTestMatcher, want: false, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			inner := ETagMatcher(func(string) (bool, error) {
				return test.inner, test.err
			})

			got, err := NegateETagMatch(inner)("etag")

			if test.wantErr {
				require.ErrorIs(t, err, errTestMatcher)
			} else {
				require.NoError(t, err)
			}

			require.Equal(t, test.want, got)
		})
	}
}

func TestAndETagMatch(t *testing.T) {
	t.Parallel()

	alwaysMatch := func(string) (bool, error) { return true, nil }
	neverMatch := func(string) (bool, error) { return false, nil }
	errorMatch := func(string) (bool, error) { return false, errTestMatcher }

	tests := []struct {
		name     string
		matchers []ETagMatcher
		want     bool
		wantErr  bool
	}{
		{name: "no matchers", matchers: nil, want: true, wantErr: false},
		{name: "all match", matchers: []ETagMatcher{alwaysMatch, alwaysMatch}, want: true, wantErr: false},
		{name: "one does not match", matchers: []ETagMatcher{alwaysMatch, neverMatch}, want: false, wantErr: false},
		{name: "none match", matchers: []ETagMatcher{neverMatch, neverMatch}, want: false, wantErr: false},
		{name: "matcher errors", matchers: []ETagMatcher{alwaysMatch, errorMatch}, want: false, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := AndETagMatch(test.matchers...)("etag")

			if test.wantErr {
				require.ErrorIs(t, err, errTestMatcher)
			} else {
				require.NoError(t, err)
			}

			require.Equal(t, test.want, got)
		})
	}

	t.Run("short circuits on the first mismatch", func(t *testing.T) {
		t.Parallel()

		calls := 0
		second := ETagMatcher(func(string) (bool, error) {
			calls++

			return true, nil
		})

		got, err := AndETagMatch(neverMatch, second)("etag")
		require.NoError(t, err)
		require.False(t, got)
		require.Zero(t, calls)
	})

	t.Run("combines with conditional match", func(t *testing.T) {
		t.Parallel()

		matcher := AndETagMatch(
			webdav.ConditionalMatch(`"etag"`).MatchETag,
			NegateETagMatch(webdav.ConditionalMatch(`"other-etag"`).MatchETag),
		)

		got, err := matcher("etag")
		require.NoError(t, err)
		require.True(t, got)

		got, err = matcher("other-etag")
		require.NoError(t, err)
		require.False(t, got)
	})

	t.Run("wildcard never matches", func(t *testing.T) {
		t.Parallel()

		matcher := AndETagMatch(
			webdav.ConditionalMatch(`"etag"`).MatchETag,
			NegateETagMatch(webdav.ConditionalMatch("*").MatchETag),
		)

		got, err := matcher("etag")
		require.NoError(t, err)
		require.False(t, got)
	})
}
