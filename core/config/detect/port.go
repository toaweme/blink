package detect

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/toaweme/blink/core/config"
)

// portEnvFiles are the dotenv-style files SniffPorts reads, in order. They are
// the highest-signal, lowest-false-positive place a dev server's port is
// written down; scanning source would guess wrong far too often.
var portEnvFiles = []string{".env", ".env.local", ".env.development", ".env.dev", ".env.example"}

// SniffPorts makes a best-effort guess at the TCP port(s) a service listens on
// by reading dotenv files in its directory (falling back to the project root).
// It looks for PORT-ish keys (PORT, HTTP_PORT, ...) and ADDR-ish keys whose
// value carries a ":port". Returns the unique ports found in first-seen order,
// or nil. It never reads source code - a wrong guess here would have blink kill
// an unrelated process before start, so the bar for a match is deliberately high.
func SniffPorts(root string, svc config.Service) []int {
	dirs := []string{filepath.Join(root, svc.Dir)}
	if svc.Dir != "" {
		dirs = append(dirs, root) // fall back to the project root for monorepo .env at the top
	}

	seen := make(map[int]bool)
	var ports []int
	for _, dir := range dirs {
		for _, name := range portEnvFiles {
			for _, p := range portsFromEnvFile(filepath.Join(dir, name)) {
				if p < 1 || p > 65535 || seen[p] {
					continue
				}
				seen[p] = true
				ports = append(ports, p)
			}
		}
		if len(ports) > 0 {
			// a port found in the service's own dir wins; don't also pull
			// unrelated ports from the root .env.
			break
		}
	}
	return ports
}

// portsFromEnvFile parses KEY=VALUE lines and extracts a port from any key that
// names a port or address. Missing files yield nil (the common case).
func portsFromEnvFile(path string) []int {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var ports []int
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.ToUpper(strings.TrimSpace(key))
		val = unquote(strings.TrimSpace(val))
		if !portKey(key) {
			continue
		}
		if p, ok := portFromValue(val); ok {
			ports = append(ports, p)
		}
	}
	return ports
}

// portKey reports whether an env var name plausibly holds a listen port or
// address: anything containing PORT, or a known address key.
func portKey(key string) bool {
	if strings.Contains(key, "PORT") {
		return true
	}
	switch key {
	case "ADDR", "LISTEN", "BIND", "HTTP_ADDR", "SERVER_ADDR", "LISTEN_ADDR":
		return true
	}
	return false
}

// portFromValue pulls a port out of a value that is either a bare number
// ("8080") or an address with a trailing ":port" (":8080", "0.0.0.0:8080",
// "http://localhost:8080"). Returns false when no plausible port is present.
func portFromValue(val string) (int, bool) {
	if val == "" {
		return 0, false
	}
	if n, err := strconv.Atoi(val); err == nil {
		return n, true
	}
	if i := strings.LastIndex(val, ":"); i >= 0 {
		tail := val[i+1:]
		// strip a trailing path or query (e.g. localhost:8080/health).
		if j := strings.IndexAny(tail, "/?#"); j >= 0 {
			tail = tail[:j]
		}
		if n, err := strconv.Atoi(tail); err == nil {
			return n, true
		}
	}
	return 0, false
}

func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
