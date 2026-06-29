//go:build darwin

package portprobe

import (
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

// listenPorts shells out to lsof (macOS base system) for the LISTEN sockets
// owned by process group pgid; macOS has no /proc and the libproc path needs
// cgo. -a ANDs the selectors "-g pgid" and "-iTCP -sTCP:LISTEN"; -nP keep hosts
// and ports numeric; -Fn emits machine-readable output with name lines prefixed
// "n".
func listenPorts(pgid int) ([]int, error) {
	//nolint:gosec,noctx // fixed, short-lived lsof probe; the only variable arg is pgid, an int formatted via strconv. No context to bind: the call returns promptly and has no lifecycle to cancel.
	out, err := exec.Command("lsof", "-nP", "-a", "-g", strconv.Itoa(pgid), "-iTCP", "-sTCP:LISTEN", "-Fn").Output()
	if err != nil {
		// lsof exits 1 when nothing matches; that's an empty set, not a failure.
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to list listeners with lsof: %w", err)
	}
	return parseLsofPorts(out), nil
}

// parseLsofPorts reads lsof -Fn output, pulling the port from each name line
// (e.g. "n*:8080", "n127.0.0.1:8080", "n[::1]:8080").
func parseLsofPorts(out []byte) []int {
	seen := make(map[int]bool)
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.HasPrefix(line, "n") {
			continue
		}
		name := line[1:]
		i := strings.LastIndex(name, ":")
		if i < 0 {
			continue
		}
		if p, err := strconv.Atoi(name[i+1:]); err == nil {
			seen[p] = true
		}
	}
	return sortedKeys(seen)
}

func sortedKeys(m map[int]bool) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}
