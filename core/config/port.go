package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Port is one entry in Service.Ports: either a literal TCP port or a reference
// to an environment variable holding the port at runtime. A literal serializes
// as a bare integer, a reference as the bare env-var name. A "${KEY}"/"$KEY"
// form is accepted on input but normalises to the bare name on write.
type Port struct { //nolint:recvcheck // UnmarshalYAML must be a pointer receiver to mutate; Marshal* must be value receivers so marshaling a non-addressable Port copy still uses them
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

// Resolve returns the concrete port number. For a literal it is Value; for a
// reference it is env[EnvKey], falling back to the process environment. ok is
// false when the value is not a valid 1-65535 port.
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

// String renders the bare env-var name for a reference, the decimal number for a literal.
func (p Port) String() string {
	if p.EnvKey != "" {
		return p.EnvKey
	}
	return strconv.Itoa(p.Value)
}

// ResolvePorts resolves every entry against env, dropping any that cannot be
// resolved to a valid port.
func ResolvePorts(ports []Port, env map[string]string) []int {
	out := make([]int, 0, len(ports))
	for _, p := range ports {
		if n, ok := p.Resolve(env); ok {
			out = append(out, n)
		}
	}
	return out
}

// ParsePort reads one textual port entry: a value that parses as an integer is
// a literal port, anything else is an env-var name. A leading "${...}"/"$..."
// sigil is stripped first, leaving only the bare name.
func ParsePort(s string) (Port, error) {
	s = strings.TrimSpace(stripEnvSigil(s))
	if s == "" {
		return Port{}, errors.New("invalid port: empty value")
	}
	if n, err := strconv.Atoi(s); err == nil {
		return Port{Value: n}, nil
	}
	return Port{EnvKey: s}, nil
}

// stripEnvSigil removes an optional "${KEY}" or "$KEY" wrapper, returning the
// inner name; a value with no sigil is returned unchanged.
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

// UnmarshalYAML decodes a literal int or an env-var name (with optional sigil) into the Port.
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

// MarshalYAML renders the Port as a bare env-var name for a reference or a bare integer for a literal.
func (p Port) MarshalYAML() (any, error) {
	if p.EnvKey != "" {
		return p.EnvKey, nil
	}
	return p.Value, nil
}

// MarshalJSON renders the Port as a JSON string for a reference or a JSON number for a literal.
func (p Port) MarshalJSON() ([]byte, error) {
	if p.EnvKey != "" {
		return json.Marshal(p.String())
	}
	return json.Marshal(p.Value)
}

// MarshalText drives TOML encoding. Every entry serializes as a string so a
// mixed literal/reference list stays a homogeneous TOML string array.
func (p Port) MarshalText() ([]byte, error) {
	return []byte(p.String()), nil
}
