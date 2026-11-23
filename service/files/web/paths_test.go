package web

import "testing"

func TestEscapePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path string
		want string
	}{
		{
			path: "/file/hello world.txt",
			want: "/file/hello%20world.txt",
		},
		{
			path: "/file/a#b.txt",
			want: "/file/a%23b.txt",
		},
		{
			path: "/file/a?b.txt",
			want: "/file/a%3Fb.txt",
		},
		{
			path: "/file/100%.txt",
			want: "/file/100%25.txt",
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
