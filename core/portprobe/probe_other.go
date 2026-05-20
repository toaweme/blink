//go:build !linux && !darwin

package portprobe

// listenPorts is the fallback for platforms blink has no port-attribution for.
// blink is a unix-only supervisor (no Windows process groups, no exec_windows),
// so runtime discovery degrades to unavailable and callers fall back to
// detect.SniffPorts' .env guess.
func listenPorts(pgid int) ([]int, error) {
	return nil, ErrUnsupported
}
