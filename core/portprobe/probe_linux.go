//go:build linux

package portprobe

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// tcpStateListen is the hex connection-state code for TCP_LISTEN in the kernel
// socket tables exposed under /proc/net.
const tcpStateListen = "0A"

// listenPorts maps the LISTEN sockets in /proc/net/tcp{,6} to the process group
// pgid by inode: it collects the socket inodes held by every pid in the group
// (via /proc/<pid>/fd) and returns the ports of the listening sockets among
// them. Reading /proc avoids depending on lsof or ss being installed.
func listenPorts(pgid int) ([]int, error) {
	inodePorts, err := listenInodePorts()
	if err != nil {
		return nil, err
	}
	if len(inodePorts) == 0 {
		return nil, nil
	}

	pids, err := pidsInGroup(pgid)
	if err != nil {
		return nil, err
	}

	seen := make(map[int]bool)
	for _, pid := range pids {
		for _, inode := range socketInodes(pid) {
			if port, ok := inodePorts[inode]; ok {
				seen[port] = true
			}
		}
	}
	out := make([]int, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Ints(out)
	return out, nil
}

// listenInodePorts returns a socket-inode -> port map for every LISTEN socket in
// the kernel TCP tables.
func listenInodePorts() (map[string]int, error) {
	out := make(map[string]int)
	for _, path := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		if err := readListenInodes(path, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func readListenInodes(path string, out map[string]int) error {
	f, err := os.Open(path)
	if err != nil {
		// tcp6 is absent on IPv6-disabled kernels; not a failure.
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read kernel socket table %q: %w", path, err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Scan() // skip the header row
	for sc.Scan() {
		// columns: sl local_address rem_address st ... uid timeout inode
		fields := strings.Fields(sc.Text())
		if len(fields) < 10 || fields[3] != tcpStateListen {
			continue
		}
		port, ok := portFromHexAddr(fields[1])
		if !ok {
			continue
		}
		out[fields[9]] = port
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("failed to scan kernel socket table %q: %w", path, err)
	}
	return nil
}

// portFromHexAddr parses the port out of a "HEXIP:HEXPORT" local-address field.
func portFromHexAddr(addr string) (int, bool) {
	i := strings.LastIndex(addr, ":")
	if i < 0 {
		return 0, false
	}
	port, err := strconv.ParseInt(addr[i+1:], 16, 32)
	if err != nil {
		return 0, false
	}
	return int(port), true
}

// pidsInGroup returns the pids whose process group is pgid, by reading the pgrp
// field of each /proc/<pid>/stat.
func pidsInGroup(pgid int) ([]int, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("failed to read /proc: %w", err)
	}
	var pids []int
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		if statPgrp(pid) == pgid {
			pids = append(pids, pid)
		}
	}
	return pids, nil
}

// statPgrp reads the process group id from /proc/<pid>/stat, or 0 on any error.
// The format is "pid (comm) state ppid pgrp ...": comm can contain spaces and
// parens, so the fixed fields are read from after the final ')'.
func statPgrp(pid int) int {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return 0
	}
	close := strings.LastIndex(string(data), ")")
	if close < 0 {
		return 0
	}
	fields := strings.Fields(string(data)[close+1:])
	// after comm: [0]=state [1]=ppid [2]=pgrp
	if len(fields) < 3 {
		return 0
	}
	pgrp, err := strconv.Atoi(fields[2])
	if err != nil {
		return 0
	}
	return pgrp
}

// socketInodes returns the socket inodes held as open fds by pid, read from the
// "socket:[inode]" symlink targets under /proc/<pid>/fd.
func socketInodes(pid int) []string {
	dir := filepath.Join("/proc", strconv.Itoa(pid), "fd")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var inodes []string
	for _, e := range entries {
		target, err := os.Readlink(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		if strings.HasPrefix(target, "socket:[") && strings.HasSuffix(target, "]") {
			inodes = append(inodes, target[len("socket:["):len(target)-1])
		}
	}
	return inodes
}
