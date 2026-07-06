//go:build windows

package portprobe

import (
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

// listenPorts is the best-effort Windows attribution. Windows has no process
// groups and the runner launches each service under cmd.exe, so there is no
// group id to query the way unix does. Instead pgid is treated as the service's
// root pid (the cmd.exe wrapper): we walk its descendant process tree from a
// Toolhelp snapshot and keep the LISTEN ports netstat attributes to any pid in
// that tree. netstat.exe ships with every Windows, so this needs no extra
// dependency. Any failure degrades to no ports (callers fall back to the .env
// guess) rather than an error, so a probe is never worse off than on a platform
// with no implementation at all.
func listenPorts(pgid int) ([]int, error) {
	tree := processTree(pgid)
	if len(tree) == 0 {
		return nil, nil
	}
	pidPorts, err := netstatListenPorts()
	if err != nil {
		//nolint:nilerr // best-effort: a failed netstat degrades to the .env fallback, not a probe error.
		return nil, nil
	}

	seen := make(map[int]bool)
	for pid := range tree {
		for _, port := range pidPorts[pid] {
			seen[port] = true
		}
	}
	out := make([]int, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Ints(out)
	return out, nil
}

// processTree returns root plus every pid descended from it, resolved from a
// single Toolhelp process snapshot. It is used in place of a unix process group:
// a service's real listener (go/node/etc.) is a child of the cmd.exe wrapper, so
// the whole subtree must be considered. Returns nil on any snapshot error.
func processTree(root int) map[int]bool {
	snapshot, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer func() { _ = syscall.CloseHandle(snapshot) }()

	var e syscall.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	if err := syscall.Process32First(snapshot, &e); err != nil {
		return nil
	}
	parents := make(map[int]int)
	for {
		parents[int(e.ProcessID)] = int(e.ParentProcessID)
		if err := syscall.Process32Next(snapshot, &e); err != nil {
			break
		}
	}

	// grow the set until no pid's parent is newly in it: a plain reachability
	// closure over the child -> parent map, small enough that O(n^2) is fine.
	tree := map[int]bool{root: true}
	for changed := true; changed; {
		changed = false
		for pid, ppid := range parents {
			if !tree[pid] && tree[ppid] {
				tree[pid] = true
				changed = true
			}
		}
	}
	return tree
}

// netstatListenPorts maps each owning pid to the TCP ports it is LISTENing on,
// parsed from `netstat -a -n -o -p TCP`. A listening row is identified by its
// wildcard foreign address (ending ":0") rather than the state word, so the
// parse is not broken by a localized Windows that prints the state in another
// language.
func netstatListenPorts() (map[int][]int, error) {
	//nolint:noctx // fixed built-in netstat probe, no user input; returns promptly and has no lifecycle to cancel.
	out, err := exec.Command("netstat", "-a", "-n", "-o", "-p", "TCP").Output()
	if err != nil {
		return nil, err
	}

	result := make(map[int][]int)
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		// columns: Proto Local-Address Foreign-Address State PID
		if len(fields) < 5 || !strings.EqualFold(fields[0], "TCP") {
			continue
		}
		if !strings.HasSuffix(fields[2], ":0") {
			continue // not a listening socket
		}
		port, ok := portFromAddr(fields[1])
		if !ok {
			continue
		}
		pid, err := strconv.Atoi(fields[4])
		if err != nil {
			continue
		}
		result[pid] = append(result[pid], port)
	}
	return result, nil
}

// portFromAddr pulls the port off a netstat local-address field, e.g.
// "0.0.0.0:8080", "127.0.0.1:8080", or "[::]:8080".
func portFromAddr(addr string) (int, bool) {
	i := strings.LastIndex(addr, ":")
	if i < 0 {
		return 0, false
	}
	port, err := strconv.Atoi(addr[i+1:])
	if err != nil {
		return 0, false
	}
	return port, true
}
