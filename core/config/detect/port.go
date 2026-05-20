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
// by reading dotenv files in its own directory (root+svc.Dir). It looks for
// PORT-ish keys (PORT, HTTP_PORT, ...) and ADDR-ish keys whose value carries a
// ":port". Returns the unique ports found in first-seen order, or nil.
//
// It reads only the service's own directory - never the project root and never
// source code. A shared monorepo root .env lists ports for many services and
// can't be attributed to any one of them, so pulling from it would tag every
// service with the same bogus list (and blink would then kill all of them on
// start). The caller (scanServices) further skips dirs shared by >1 service.
func SniffPorts(root string, svc config.Service) []config.Port {
	dir := filepath.Join(root, svc.Dir)
	seen := make(map[int]bool)
	var ports []config.Port
	for _, name := range portEnvFiles {
		for _, ep := range portsFromEnvFile(filepath.Join(dir, name)) {
			if ep.port < 1 || ep.port > 65535 || seen[ep.port] {
				continue
			}
			seen[ep.port] = true
			ports = append(ports, config.LiteralPort(ep.port))
		}
	}
	return ports
}

// EnvKeyForPort reports the env var in the service's own .env files whose value
// is the given port, if any. It powers `blink init`'s runtime probe: once a
// service is observed binding a port, blink prefers writing the env-var name over the
// bare number so the config tracks the .env instead of hardcoding it. The first
// matching key (in portEnvFiles, then file order) wins.
func EnvKeyForPort(root string, svc config.Service, port int) (string, bool) {
	dir := filepath.Join(root, svc.Dir)
	for _, name := range portEnvFiles {
		for _, ep := range portsFromEnvFile(filepath.Join(dir, name)) {
			if ep.port == port {
				return ep.key, true
			}
		}
	}
	return "", false
}

// envPort pairs the env var name with the port parsed from its value, so the
// caller can both list ports (SniffPorts) and map a port back to its key
// (EnvKeyForPort).
type envPort struct {
	key  string
	port int
}

// portsFromEnvFile parses KEY=VALUE lines and extracts a port from any key that
// names a port or address. Missing files yield nil (the common case).
func portsFromEnvFile(path string) []envPort {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var ports []envPort
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
			ports = append(ports, envPort{key: key, port: p})
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
