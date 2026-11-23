package web

import "testing"

func TestRelativePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		value string
		want  string
		ok    bool
	}{
		{value: "a.txt", want: "a.txt", ok: true},
		{value: "dir/a.txt", want: "dir/a.txt", ok: true},
		{value: "dir/../a.txt", want: "a.txt", ok: true},
		{value: "", ok: false},
		{value: ".", ok: false},
		{value: "..", ok: false},
		{value: "../a.txt", ok: false},
		{value: "/a.txt", ok: false},
	}

	for _, test := range tests {
		got, err := relativePath(test.value)
		if test.ok {
			if err != nil {
				t.Errorf("relativePath(%q) error = %v", test.value, err)
			} else if got != test.want {
				t.Errorf("relativePath(%q) = %q, want %q", test.value, got, test.want)
			}
		} else if err == nil {
			t.Errorf("relativePath(%q) = %q, want error", test.value, got)
		}
	}
}
