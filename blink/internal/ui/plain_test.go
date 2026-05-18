package ui

import (
	"bytes"
	"testing"
)

func Test_CRLFWriter(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "bare lf gets cr", in: "line\n", want: "line\r\n"},
		{name: "multiple lines", in: "a\nb\nc\n", want: "a\r\nb\r\nc\r\n"},
		{name: "existing crlf untouched", in: "a\r\nb\r\n", want: "a\r\nb\r\n"},
		{name: "no newline", in: "partial", want: "partial"},
		{name: "leading lf", in: "\nx", want: "\r\nx"},
		{name: "lone cr untouched", in: "a\rb", want: "a\rb"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			w := &crlfWriter{w: &buf}
			n, err := w.Write([]byte(tt.in))
			if err != nil {
				t.Fatalf("Write failed: %v", err)
			}
			if n != len(tt.in) {
				t.Fatalf("Write returned n=%d, want %d (original length)", n, len(tt.in))
			}
			if got := buf.String(); got != tt.want {
				t.Fatalf("Write(%q) wrote %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
