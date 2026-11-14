package webdav

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testETag = "abc"

func TestParseEntityTag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
		want  entityTag
		rest  string
		err   bool
	}{
		{
			name:  "strong",
			value: `"abc"`,
			want:  entityTag{value: testETag},
		},
		{
			name:  "weak",
			value: `W/"abc"`,
			want:  entityTag{value: testETag, weak: true},
		},
		{
			name:  "empty opaque tag",
			value: `""`,
			want:  entityTag{},
		},
		{
			name:  "comma in opaque tag",
			value: `"a,b"`,
			want:  entityTag{value: "a,b"},
		},
		{
			name:  "rest after tag",
			value: `"abc", "def"`,
			want:  entityTag{value: testETag},
			rest:  `, "def"`,
		},
		{
			name:  "rest after weak tag",
			value: `W/"abc",`,
			want:  entityTag{value: testETag, weak: true},
			rest:  ",",
		},
		{
			name:  "missing quotes",
			value: testETag,
			err:   true,
		},
		{
			name:  "missing closing quote",
			value: `"abc`,
			err:   true,
		},
		{
			name:  "weak marker without tag",
			value: "W/",
			err:   true,
		},
		{
			name:  "empty",
			value: "",
			err:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tag, rest, err := parseEntityTag(tt.value)
			if tt.err {
				require.ErrorIs(t, err, ErrInvalidEntityTag)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, tag)
			assert.Equal(t, tt.rest, rest)
		})
	}
}

func TestParseEntityTags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
		want  []entityTag
		err   bool
	}{
		{
			name:  "empty",
			value: "",
		},
		{
			name:  "single strong tag",
			value: `"abc"`,
			want:  []entityTag{{value: testETag}},
		},
		{
			name:  "single weak tag",
			value: `W/"abc"`,
			want:  []entityTag{{value: testETag, weak: true}},
		},
		{
			name:  "strong tag list",
			value: `"xyzzy", "r2d2xxxx", "c3piozzzz"`,
			want: []entityTag{
				{value: "xyzzy"},
				{value: "r2d2xxxx"},
				{value: "c3piozzzz"},
			},
		},
		{
			name:  "weak tag list",
			value: `W/"xyzzy", W/"r2d2xxxx", W/"c3piozzzz"`,
			want: []entityTag{
				{value: "xyzzy", weak: true},
				{value: "r2d2xxxx", weak: true},
				{value: "c3piozzzz", weak: true},
			},
		},
		{
			name:  "mixed tag list",
			value: `W/"abc", "def"`,
			want: []entityTag{
				{value: testETag, weak: true},
				{value: "def"},
			},
		},
		{
			name:  "whitespace around tags",
			value: ` "abc" , W/"def" `,
			want: []entityTag{
				{value: testETag},
				{value: "def", weak: true},
			},
		},
		{
			name:  "comma in opaque tag",
			value: `"a,b"`,
			want:  []entityTag{{value: "a,b"}},
		},
		{
			name:  "trailing comma",
			value: `"abc",`,
			want:  []entityTag{{value: testETag}},
		},
		{
			name:  "empty list elements",
			value: `"abc",, "def"`,
			want: []entityTag{
				{value: testETag},
				{value: "def"},
			},
		},
		{
			name:  "missing comma",
			value: `"abc" "def"`,
			err:   true,
		},
		{
			name:  "malformed tag",
			value: `"abc", def`,
			err:   true,
		},
		{
			name:  "wildcard is not an entity tag",
			value: "*",
			err:   true,
		},
		{
			name:  "wildcard mixed with tags",
			value: `*, "abc"`,
			err:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tags, err := parseEntityTags(tt.value)
			if tt.err {
				require.ErrorIs(t, err, ErrInvalidEntityTag)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, tags)
		})
	}
}

func TestConditionalMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		match  ConditionalMatch
		isSet  bool
		isWild bool
		etag   string
		strong bool
		weak   bool
		err    bool
	}{
		{
			name:  "empty",
			match: "",
			etag:  testETag,
		},
		{
			name:   "wildcard",
			match:  "*",
			isSet:  true,
			isWild: true,
			etag:   testETag,
			strong: true,
			weak:   true,
		},
		{
			name:   "wildcard with whitespace",
			match:  " * ",
			isSet:  true,
			isWild: true,
			etag:   testETag,
			strong: true,
			weak:   true,
		},
		{
			name:   "strong etag matches",
			match:  `"abc"`,
			isSet:  true,
			etag:   testETag,
			strong: true,
			weak:   true,
		},
		{
			name:  "strong etag mismatch",
			match: `"abc"`,
			isSet: true,
			etag:  "def",
		},
		{
			name:   "list with second match",
			match:  `"abc", "def"`,
			isSet:  true,
			etag:   "def",
			strong: true,
			weak:   true,
		},
		{
			name:  "list without match",
			match: `"abc", "def"`,
			isSet: true,
			etag:  "ghi",
		},
		{
			name:  "weak etag with strong comparison",
			match: `W/"abc"`,
			isSet: true,
			etag:  testETag,
			weak:  true,
		},
		{
			name:   "weak tag before strong tag in the same list",
			match:  `W/"abc", "abc"`,
			isSet:  true,
			etag:   testETag,
			strong: true,
			weak:   true,
		},
		{
			name:  "malformed etag",
			match: testETag,
			isSet: true,
			etag:  testETag,
			err:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.isSet, tt.match.IsSet())
			assert.Equal(t, tt.isWild, tt.match.IsWildcard())

			matched, err := tt.match.MatchETag(tt.etag)
			if tt.err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tt.strong, matched)

			matched, err = tt.match.MatchETagWeak(tt.etag)
			if tt.err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tt.weak, matched)
		})
	}
}

func TestConditionalHeaders(t *testing.T) {
	t.Parallel()

	header := http.Header{}
	header.Add("If-Match", `"abc"`)
	header.Add("If-Match", `W/"def"`)
	header.Add("If-None-Match", `"ghi"`)
	header.Add("If-None-Match", `"jkl"`)

	assert.Equal(t, ConditionalMatch(`"abc",W/"def"`), IfMatch(header))
	assert.Equal(t, ConditionalMatch(`"ghi","jkl"`), IfNoneMatch(header))
	assert.False(t, IfMatch(http.Header{}).IsSet())
	assert.False(t, IfNoneMatch(http.Header{}).IsSet())
}
