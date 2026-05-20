// Package portprobe lists the local TCP ports a given process group is
// listening on, so `blink init` can discover the port a freshly started service
// bound by asking "which ports does this service's process group own?". This
// per-group attribution (rather than a global before/after diff) lets several
// services be probed concurrently without stealing each other's ports.
//
// Each supported OS has its own listenPorts() in a build-tagged file: Linux
// matches /proc sockets to the group's pids (no external binary), macOS shells
// out to `lsof -g` (base system). blink is a unix-only supervisor, so other
// platforms return ErrUnsupported.
package portprobe

import "errors"

// ErrUnsupported is returned on platforms without a port-attribution
// implementation. Callers treat it as "runtime discovery unavailable here" and
// fall back to detect.SniffPorts' .env guess.
var ErrUnsupported = errors.New("listening-port discovery is not supported on this platform")

// ListenPorts returns the local TCP ports in the LISTEN state owned by any
// process in the group pgid, ascending. blink starts each service in its own
// process group (Setpgid), so pgid is the service runner's pid.
func ListenPorts(pgid int) ([]int, error) {
	return listenPorts(pgid)
}
