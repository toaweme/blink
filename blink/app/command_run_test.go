package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/toaweme/blink/core/config"
)

// configDirWith returns a temp dir holding a blink.yaml built from the given
// top-level lines prepended to the minimal service block, so a test can set
// fields like zen: true and take the normal load path.
func configDirWith(t *testing.T, top string) string {
	t.Helper()
	dir := t.TempDir()
	yaml := top + "services:\n  - name: web\n    commands:\n      run:\n        command: ./bin/web\n        service: true\n"
	if err := os.WriteFile(filepath.Join(dir, "blink.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatalf("write blink.yaml: %v", err)
	}
	return dir
}

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

func Test_LoadRunConfig_ZenOverride(t *testing.T) {
	tests := []struct {
		name      string
		configTop string // extra top-level yaml lines
		flag      bool   // -z / BLINK_ZEN
		want      bool
	}{
		{name: "config zen honored when flag unset", configTop: "zen: true\n", flag: false, want: true},
		{name: "flag on overrides config off", configTop: "", flag: true, want: true},
		{name: "flag on with config on stays on", configTop: "zen: true\n", flag: true, want: true},
		{name: "both unset stays off", configTop: "", flag: false, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := loadRunConfig(configDirWith(t, tt.configTop), RunConfig{Zen: tt.flag})
			if err != nil {
				t.Fatalf("loadRunConfig failed: %v", err)
			}
			if cfg.Zen != tt.want {
				t.Fatalf("Zen = %v, want %v (configTop %q, flag %v)", cfg.Zen, tt.want, tt.configTop, tt.flag)
			}
		})
	}
}

func Test_LoadRunConfig_InvalidOverride(t *testing.T) {
	tests := []struct {
		name    string
		in      RunConfig
		wantErr bool
	}{
		{name: "empty is unset", in: RunConfig{}, wantErr: false},
		{name: "force_shutdown on", in: RunConfig{ForceShutdown: "on"}, wantErr: false},
		{name: "force_shutdown off", in: RunConfig{ForceShutdown: "off"}, wantErr: false},
		{name: "force_shutdown invalid", in: RunConfig{ForceShutdown: "enabled"}, wantErr: true},
		{name: "logs on", in: RunConfig{Logs: "on"}, wantErr: false},
		{name: "logs off", in: RunConfig{Logs: "off"}, wantErr: false},
		{name: "logs invalid", in: RunConfig{Logs: "1"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadRunConfig(configDirWith(t, ""), tt.in)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidFlag) {
					t.Fatalf("loadRunConfig err = %v, want ErrInvalidFlag", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("loadRunConfig failed: %v", err)
			}
		})
	}
}

func Test_EnabledServices(t *testing.T) {
	all := []config.Service{
		{Name: "web"},
		{Name: "worker", Disabled: true},
		{Name: "db"},
	}
	got := enabledServices(all)
	if len(got) != 2 || got[0].Name != "web" || got[1].Name != "db" {
		t.Fatalf("enabledServices dropped the wrong entries: %+v", got)
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
