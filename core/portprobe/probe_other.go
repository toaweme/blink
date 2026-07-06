//go:build !linux && !darwin && !windows

package portprobe

// listenPorts is the fallback for platforms blink has no port-attribution for
// (the BSDs and anything else that is neither linux, darwin, nor windows).
// Runtime discovery degrades to unavailable and callers fall back to
// detect.SniffPorts' .env guess.
func listenPorts(pgid int) ([]int, error) {
	return nil, ErrUnsupported
}
