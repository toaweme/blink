// Package portkill terminates whatever process is listening on a given TCP port. The supervisor uses it before starting a service that declares Ports, since a previous run's child may linger and block the new bind. Best-effort: failures (no permission, lsof not installed) are returned but never block service start.
package portkill

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Kill terminates every process listening on any of the given TCP ports. It sends SIGTERM, waits briefly, then escalates to SIGKILL on stragglers. The current pid and its parent are skipped so blink can't kill itself. Returns the signaled PIDs and the first non-fatal error encountered. A missing `lsof` binary returns ErrLsofMissing.
func Kill(ports []int) ([]int, error) {
	if len(ports) == 0 {
		return nil, nil
	}
	pids, err := Listeners(ports)
	if err != nil {
		return nil, err
	}
	if len(pids) == 0 {
		return nil, nil
	}
	self := os.Getpid()
	parent := os.Getppid()
	var signaled []int
	for _, pid := range pids {
		if pid == self || pid == parent {
			continue
		}
		proc, err := os.FindProcess(pid)
		if err != nil {
			continue
		}
		_ = proc.Signal(syscall.SIGTERM)
		signaled = append(signaled, pid)
	}
	if len(signaled) == 0 {
		return nil, nil
	}
	// give SIGTERM a moment to settle, then SIGKILL anything still alive.
	time.Sleep(150 * time.Millisecond)
	for _, pid := range signaled {
		if alive(pid) {
			proc, err := os.FindProcess(pid)
			if err != nil {
				continue
			}
			_ = proc.Signal(syscall.SIGKILL)
		}
	}
	return signaled, nil
}

// ErrLsofMissing is returned when the system has no `lsof` binary. Port scanning is best-effort; the supervisor falls back to letting the service's own bind error surface.
var ErrLsofMissing = errors.New("lsof not found in PATH")

// Listeners returns the PIDs holding LISTEN sockets on any of the given ports. It shells out to `lsof`, the portable (macOS and Linux) way to ask which process owns a port without parsing /proc on Linux and a different format on darwin.
func Listeners(ports []int) ([]int, error) {
	if _, err := exec.LookPath("lsof"); err != nil {
		return nil, ErrLsofMissing
	}
	args := []string{"-nP", "-sTCP:LISTEN", "-t"}
	for _, p := range ports {
		args = append(args, "-iTCP:"+strconv.Itoa(p))
	}
	//nolint:noctx // fixed, short-lived lsof scan with no lifecycle to cancel; args are numeric ports formatted via strconv.
	cmd := exec.Command("lsof", args...)
	out, err := cmd.Output()
	// lsof exits 1 when nothing matches; that isn't an error here.
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 1 && len(out) == 0 {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to scan listeners with lsof: %w", err)
	}
	seen := make(map[int]struct{})
	var pids []int
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		pid, err := strconv.Atoi(line)
		if err != nil {
			continue
		}
		if _, ok := seen[pid]; ok {
			continue
		}
		seen[pid] = struct{}{}
		pids = append(pids, pid)
	}
	return pids, nil
}

// alive reports whether the given pid still exists. It sends signal 0, the standard existence probe: no signal is delivered, but the call succeeds only if the process exists and is signal-reachable.
func alive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}
