package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/toaweme/blink/core/config"
)

// configDir returns a temp dir holding a minimal valid blink.yaml so
// loadRunConfig takes the normal load path (not zero-config detection).
func configDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	yaml := "services:\n  - name: web\n    commands:\n      run:\n        command: ./bin/web\n        service: true\n"
	if err := os.WriteFile(filepath.Join(dir, "blink.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatalf("write blink.yaml: %v", err)
	}
	return dir
}

func Test_LoadRunConfig_LogsOverride(t *testing.T) {
	tests := []struct {
		name string
		flag string
		want bool // LogWriteEnabled()
	}{
		{name: "default unset is on", flag: "", want: true},
		{name: "off disables", flag: "off", want: false},
		{name: "false disables", flag: "false", want: false},
		{name: "on enables", flag: "on", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := loadRunConfig(configDir(t), RunConfig{Logs: tt.flag})
			if err != nil {
				t.Fatalf("loadRunConfig failed: %v", err)
			}
			if got := cfg.LogWriteEnabled(); got != tt.want {
				t.Fatalf("LogWriteEnabled() = %v, want %v (flag %q)", got, tt.want, tt.flag)
			}
		})
	}
}

func Test_ScopeServices(t *testing.T) {
	all := []config.Service{{Name: "web"}, {Name: "api"}, {Name: "db"}}

	tests := []struct {
		name    string
		want    []string
		expect  []string
		wantErr bool
	}{
		{name: "single", want: []string{"api"}, expect: []string{"api"}},
		{name: "subset preserves request order", want: []string{"db", "web"}, expect: []string{"db", "web"}},
		{name: "trailing empty tolerated", want: []string{"web", ""}, expect: []string{"web"}},
		{name: "doubled empty tolerated", want: []string{"web", "", "api"}, expect: []string{"web", "api"}},
		{name: "all empty scopes to nothing", want: []string{"", ""}, expect: []string{}},
		{name: "unknown name errors", want: []string{"web", "nope"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := scopeServices(all, tt.want)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("scopeServices(%v) = nil error, want error", tt.want)
				}
				return
			}
			if err != nil {
				t.Fatalf("scopeServices(%v) failed: %v", tt.want, err)
			}
			if len(got) != len(tt.expect) {
				t.Fatalf("scopeServices(%v) = %d services, want %d", tt.want, len(got), len(tt.expect))
			}
			for i, name := range tt.expect {
				if got[i].Name != name {
					t.Fatalf("service %d = %q, want %q", i, got[i].Name, name)
				}
			}
		})
	}
}
