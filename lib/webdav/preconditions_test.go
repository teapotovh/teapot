package webdav

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	daverr "github.com/teapotovh/teapot/lib/webdav/error"
)

const quotedOtherETag = `"def"`

func TestPreconditionsFromHeader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		headers map[string][]string
		want    Preconditions
		err     bool
	}{
		{
			name: "empty",
			want: Preconditions{},
		},
		{
			name: "valid",
			headers: map[string][]string{
				"If-Match":      {`"abc", "def"`},
				"If-None-Match": {`W/"ghi"`},
			},
			want: Preconditions{
				IfMatch:     `"abc", "def"`,
				IfNoneMatch: `W/"ghi"`,
			},
		},
		{
			name: "multiple header values",
			headers: map[string][]string{
				"If-Match": {testQuotedETag, quotedOtherETag},
			},
			want: Preconditions{
				IfMatch: `"abc","def"`,
			},
		},
		{
			name: "malformed if-match",
			headers: map[string][]string{
				"If-Match": {testETag},
			},
			err: true,
		},
		{
			name: "malformed if-none-match",
			headers: map[string][]string{
				"If-None-Match": {testETag},
			},
			err: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			header := http.Header(tt.headers)

			pre, err := PreconditionsFromHeader(header)
			if tt.err {
				require.Error(t, err)

				httpErr := &daverr.HTTPError{}
				require.ErrorAs(t, err, &httpErr)
				assert.Equal(t, http.StatusBadRequest, httpErr.Code)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, pre)
		})
	}
}

func TestPreconditionsEvaluate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pre     Preconditions
		exists  bool
		etag    string
		outcome Outcome
		err     bool
	}{
		{
			name:    "no preconditions",
			exists:  true,
			etag:    testETag,
			outcome: Proceed,
		},
		{
			name:    "no preconditions on missing object",
			outcome: Proceed,
		},
		{
			name:    "if-match matches",
			pre:     Preconditions{IfMatch: testQuotedETag},
			exists:  true,
			etag:    testETag,
			outcome: Proceed,
		},
		{
			name:    "if-match does not match",
			pre:     Preconditions{IfMatch: quotedOtherETag},
			exists:  true,
			etag:    testETag,
			outcome: PreconditionFailed,
		},
		{
			name:    "if-match on missing object",
			pre:     Preconditions{IfMatch: testQuotedETag},
			outcome: PreconditionFailed,
		},
		{
			name:    "if-match wildcard on existing object",
			pre:     Preconditions{IfMatch: "*"},
			exists:  true,
			etag:    testETag,
			outcome: Proceed,
		},
		{
			name:    "if-match wildcard on missing object",
			pre:     Preconditions{IfMatch: "*"},
			outcome: PreconditionFailed,
		},
		{
			name:    "if-match weak tag with strong comparison",
			pre:     Preconditions{IfMatch: `W/"abc"`},
			exists:  true,
			etag:    testETag,
			outcome: PreconditionFailed,
		},
		{
			name:    "if-none-match matches",
			pre:     Preconditions{IfNoneMatch: testQuotedETag},
			exists:  true,
			etag:    testETag,
			outcome: NotModified,
		},
		{
			name:    "if-none-match weak tag matches",
			pre:     Preconditions{IfNoneMatch: `W/"abc"`},
			exists:  true,
			etag:    testETag,
			outcome: NotModified,
		},
		{
			name:    "if-none-match does not match",
			pre:     Preconditions{IfNoneMatch: quotedOtherETag},
			exists:  true,
			etag:    testETag,
			outcome: Proceed,
		},
		{
			name:    "if-none-match wildcard on existing object",
			pre:     Preconditions{IfNoneMatch: "*"},
			exists:  true,
			etag:    testETag,
			outcome: NotModified,
		},
		{
			name:    "if-none-match wildcard on missing object",
			pre:     Preconditions{IfNoneMatch: "*"},
			outcome: Proceed,
		},
		{
			name:    "if-none-match on missing object",
			pre:     Preconditions{IfNoneMatch: testQuotedETag},
			outcome: Proceed,
		},
		{
			name: "if-match and if-none-match match",
			pre: Preconditions{
				IfMatch:     testQuotedETag,
				IfNoneMatch: testQuotedETag,
			},
			exists:  true,
			etag:    testETag,
			outcome: NotModified,
		},
		{
			name: "if-match fails before if-none-match",
			pre: Preconditions{
				IfMatch:     quotedOtherETag,
				IfNoneMatch: testQuotedETag,
			},
			exists:  true,
			etag:    testETag,
			outcome: PreconditionFailed,
		},
		{
			name:    "malformed value",
			pre:     Preconditions{IfMatch: testETag},
			exists:  true,
			etag:    testETag,
			outcome: Proceed,
			err:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			outcome, err := tt.pre.Evaluate(tt.exists, tt.etag)
			if tt.err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tt.outcome, outcome)

			err = tt.pre.CheckWrite(tt.exists, tt.etag)
			switch {
			case tt.err:
				require.Error(t, err)
			case tt.outcome != Proceed:
				require.ErrorIs(t, err, ErrPreconditionFailed)
			default:
				require.NoError(t, err)
			}
		})
	}
}
