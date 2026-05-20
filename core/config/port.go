package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Port is one entry in Service.Ports. It is either a literal TCP port (9090) or
// a reference to an environment variable that holds the port at runtime. The
// reference form keeps the number out of blink.yaml so a single .env drives both
// the service and blink's reclaim-before-start: `blink init` writes it whenever
// runtime port discovery finds the bound port already named by an env key.
//
// In blink.yaml a literal serialises as a bare integer and a reference as the
// bare env-var name, so a mixed list reads naturally with no sigil noise:
//
//	ports:
//	  - SERVER_HTTP_PORT
//	  - 9090
//
// A "${KEY}"/"$KEY" form is still accepted on input (handy when pasting from a
// compose file) but normalises to the bare name on write.
type Port struct {
	// Value is the literal port; meaningful only when EnvKey is empty.
	Value int
	// EnvKey, when non-empty, names the env var the port is read from at
	// runtime, and Value is ignored.
	EnvKey string
}

// LiteralPort builds a literal Port.
func LiteralPort(p int) Port { return Port{Value: p} }

// EnvPort builds an env-referenced Port.
func EnvPort(key string) Port { return Port{EnvKey: key} }

// Resolve returns the concrete port number. For a literal it's Value; for a
// reference it's the parsed value of env[EnvKey], falling back to the process
// environment (blink auto-loads the root .env into it at startup). ok is false
// when a reference can't be resolved to a valid 1-65535 port, so callers
// (portkill) skip it rather than acting on 0.
func (p Port) Resolve(env map[string]string) (int, bool) {
	if p.EnvKey == "" {
		return p.Value, p.Value >= 1 && p.Value <= 65535
	}
	raw := env[p.EnvKey]
	if raw == "" {
		raw = os.Getenv(p.EnvKey)
	}
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < 1 || n > 65535 {
		return 0, false
	}
	return n, true
}

// String renders the port for display and config output: the bare env-var name
// for a reference, the decimal number for a literal.
func (p Port) String() string {
	if p.EnvKey != "" {
		return p.EnvKey
	}
	return strconv.Itoa(p.Value)
}

// ResolvePorts resolves every entry against env, dropping any that can't be
// resolved to a valid port. It's the bridge for consumers that need concrete
// numbers (portkill's reclaim sweep) from a possibly env-referenced list.
func ResolvePorts(ports []Port, env map[string]string) []int {
	out := make([]int, 0, len(ports))
	for _, p := range ports {
		if n, ok := p.Resolve(env); ok {
			out = append(out, n)
		}
	}
	return out
}

// ParsePort reads one textual port entry. The rule is simple: a value that
// parses as an integer is a literal port, anything else is an env-var name. A
// leading "${...}"/"$..." sigil is stripped first so compose-style references
// are accepted, but the result stores only the bare name.
func ParsePort(s string) (Port, error) {
	s = strings.TrimSpace(stripEnvSigil(s))
	if s == "" {
		return Port{}, fmt.Errorf("invalid port: empty value")
	}
	if n, err := strconv.Atoi(s); err == nil {
		return Port{Value: n}, nil
	}
	return Port{EnvKey: s}, nil
}

// stripEnvSigil removes an optional "${KEY}" or "$KEY" wrapper, returning the
// inner name. A value with no sigil is returned unchanged.
func stripEnvSigil(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "${") && strings.HasSuffix(s, "}") {
		return s[2 : len(s)-1]
	}
	if strings.HasPrefix(s, "$") && len(s) > 1 {
		return s[1:]
	}
	return s
}

func (p *Port) UnmarshalYAML(node *yaml.Node) error {
	var i int
	if err := node.Decode(&i); err == nil {
		p.Value, p.EnvKey = i, ""
		return nil
	}
	var s string
	if err := node.Decode(&s); err != nil {
		return fmt.Errorf("failed to decode port %q: %w", node.Value, err)
	}
	parsed, err := ParsePort(s)
	if err != nil {
		return err
	}
	*p = parsed
	return nil
}

func (p Port) MarshalYAML() (any, error) {
	if p.EnvKey != "" {
		return p.EnvKey, nil
	}
	return p.Value, nil
}

func (p Port) MarshalJSON() ([]byte, error) {
	if p.EnvKey != "" {
		return json.Marshal(p.String())
	}
	return json.Marshal(p.Value)
}

// MarshalText drives TOML encoding (go-toml honours encoding.TextMarshaler).
// Every entry serialises as a string so a mixed literal/reference list stays a
// homogeneous TOML string array rather than an illegal mixed-type one.
func (p Port) MarshalText() ([]byte, error) {
	return []byte(p.String()), nil
}
