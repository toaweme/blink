// Package portkill terminates whatever process is currently listening on a
// given TCP port. It's used by the supervisor before starting a service that
// declares Ports - a previous run's child may be lingering and would block
// the new bind, so we proactively reclaim the port.
//
// This is best-effort: failures (e.g. no permission, lsof not installed) are
// returned but never block service start. Callers usually log and proceed.
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

// Kill terminates every process listening on any of the given TCP ports. It
// sends SIGTERM, waits briefly, then escalates to SIGKILL on stragglers.
// Processes owned by the current pid (or its parent - i.e. ourselves) are
// skipped so blink can't kill itself.
//
// Returns the list of PIDs that were signaled and the first non-fatal error
// encountered while collecting them. A missing `lsof` binary returns
// ErrLsofMissing - callers can treat that as "feature unavailable on this
// system" rather than as a hard failure.
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
	// give SIGTERM a moment to settle, then SIGKILL anything still alive
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

// ErrLsofMissing is returned when the system has no `lsof` binary. Port
// scanning is a best-effort convenience; the supervisor falls back to letting
// the service's own bind error surface instead.
var ErrLsofMissing = errors.New("lsof not found in PATH")

// Listeners returns the PIDs holding LISTEN sockets on any of the given ports.
// Implementation note: we shell out to `lsof` because it's the only portable
// (macOS + Linux) way to ask the kernel "who owns port N?" without parsing
// /proc on Linux and a different format on darwin.
func Listeners(ports []int) ([]int, error) {
	if _, err := exec.LookPath("lsof"); err != nil {
		return nil, ErrLsofMissing
	}
	args := []string{"-nP", "-sTCP:LISTEN", "-t"}
	for _, p := range ports {
		args = append(args, "-iTCP:"+strconv.Itoa(p))
	}
	cmd := exec.Command("lsof", args...)
	out, err := cmd.Output()
	// lsof exits 1 when nothing matches; that isn't an error for us.
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

// alive returns true when the given pid still accepts signals - i.e. it
// hasn't exited yet. We send signal 0, which is the standard "is it there?"
// probe (no actual signal delivered, but the call succeeds iff the process
// exists and is signal-reachable from us).
func alive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}
