package tui

import "testing"

func Test_ShortProjectPath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"deep", "/Users/ignas/Code/golang/src/github.com/toaweme/blink", "toaweme/blink"},
		{"two segments", "/work/api", "work/api"},
		{"single segment", "/blink", "blink"},
		{"trailing slash", "/work/api/", "work/api"},
		{"relative two", "toaweme/blink", "toaweme/blink"},
		{"bare name", "blink", "blink"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := shortProjectPath(tc.in); got != tc.want {
				t.Fatalf("shortProjectPath(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func Test_TruncLeft(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		limit int
		want  string
	}{
		{"fits", "toaweme/blink", 13, "toaweme/blink"},
		{"clips", "toaweme/blink", 6, "…blink"},
		{"exact", "blink", 5, "blink"},
		{"one column", "blink", 1, "…"},
		{"zero", "blink", 0, ""},
		{"negative", "blink", -3, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := truncLeft(tc.in, tc.limit); got != tc.want {
				t.Fatalf("truncLeft(%q, %d) = %q, want %q", tc.in, tc.limit, got, tc.want)
			}
		})
	}
}
