//go:build darwin

package portprobe

import "testing"

func Test_parseLsofPorts(t *testing.T) {
	// lsof -Fn emits one field per line; only "n" lines carry the name.
	out := []byte("p123\nn*:8080\nf3\nn127.0.0.1:5432\nn[::1]:9090\nnsomething-without-port\n")
	got := parseLsofPorts(out)
	want := []int{5432, 8080, 9090} // sorted ascending
	if len(got) != len(want) {
		t.Fatalf("parseLsofPorts = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("parseLsofPorts = %v, want %v", got, want)
		}
	}
}
