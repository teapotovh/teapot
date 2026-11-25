package web

import "testing"

func TestEscapePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path string
		want string
	}{
		{
			path: "/download/hello world.txt",
			want: "/download/hello%20world.txt",
		},
		{
			path: "/download/a#b.txt",
			want: "/download/a%23b.txt",
		},
		{
			path: "/download/a?b.txt",
			want: "/download/a%3Fb.txt",
		},
		{
			path: "/download/100%.txt",
			want: "/download/100%25.txt",
		},
		{
			path: "/browse/café/",
			want: "/browse/caf%C3%A9/",
		},
	}

	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			t.Parallel()

			if got := EscapePath(test.path); got != test.want {
				t.Errorf("EscapePath(%q) = %q, want %q", test.path, got, test.want)
			}
		})
	}
}
