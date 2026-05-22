package config

import (
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"
)

func Test_Port_YAMLRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want Port
		out  string // re-marshaled form
	}{
		{name: "literal int", in: "8080", want: LiteralPort(8080), out: "8080\n"},
		{name: "bare env name", in: "SERVER_HTTP_PORT", want: EnvPort("SERVER_HTTP_PORT"), out: "SERVER_HTTP_PORT\n"},
		{name: "braced ref normalises", in: "${PORT}", want: EnvPort("PORT"), out: "PORT\n"},
		{name: "dollar ref normalises", in: "$PORT", want: EnvPort("PORT"), out: "PORT\n"},
		{name: "quoted literal", in: `"9090"`, want: LiteralPort(9090), out: "9090\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got Port
			if err := yaml.Unmarshal([]byte(tt.in), &got); err != nil {
				t.Fatalf("unmarshal %q: %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("unmarshal %q = %+v, want %+v", tt.in, got, tt.want)
			}
			data, err := yaml.Marshal(got)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(data) != tt.out {
				t.Fatalf("marshal = %q, want %q", data, tt.out)
			}
		})
	}
}

func Test_ParsePort(t *testing.T) {
	if p, err := ParsePort("8080"); err != nil || p != LiteralPort(8080) {
		t.Fatalf("ParsePort(8080) = (%+v, %v), want literal 8080", p, err)
	}
	if p, err := ParsePort("MY_PORT"); err != nil || p != EnvPort("MY_PORT") {
		t.Fatalf("ParsePort(MY_PORT) = (%+v, %v), want env MY_PORT", p, err)
	}
	if _, err := ParsePort("   "); err == nil {
		t.Fatal("ParsePort(blank) = nil error, want error")
	}
}

func Test_Port_MarshalJSON(t *testing.T) {
	tests := []struct {
		port Port
		want string
	}{
		{LiteralPort(8080), "8080"},
		{EnvPort("PORT"), `"PORT"`},
	}
	for _, tt := range tests {
		data, err := json.Marshal(tt.port)
		if err != nil {
			t.Fatalf("marshal %v: %v", tt.port, err)
		}
		if string(data) != tt.want {
			t.Fatalf("MarshalJSON(%v) = %s, want %s", tt.port, data, tt.want)
		}
	}
}

func Test_Port_Resolve(t *testing.T) {
	env := map[string]string{"PORT": "8080", "BAD": "notnum", "ZERO": "0"}
	tests := []struct {
		name string
		port Port
		want int
		ok   bool
	}{
		{name: "literal", port: LiteralPort(9090), want: 9090, ok: true},
		{name: "literal out of range", port: LiteralPort(99999), ok: false},
		{name: "ref hit", port: EnvPort("PORT"), want: 8080, ok: true},
		{name: "ref missing", port: EnvPort("NOPE"), ok: false},
		{name: "ref non-numeric", port: EnvPort("BAD"), ok: false},
		{name: "ref zero", port: EnvPort("ZERO"), ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.port.Resolve(env)
			if ok != tt.ok || (ok && got != tt.want) {
				t.Fatalf("Resolve() = (%d, %v), want (%d, %v)", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func Test_ResolvePorts_DropsUnresolvable(t *testing.T) {
	env := map[string]string{"PORT": "8080"}
	ports := []Port{LiteralPort(9090), EnvPort("PORT"), EnvPort("MISSING")}
	got := ResolvePorts(ports, env)
	want := []int{9090, 8080}
	if len(got) != len(want) {
		t.Fatalf("ResolvePorts = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("ResolvePorts = %v, want %v", got, want)
		}
	}
}
