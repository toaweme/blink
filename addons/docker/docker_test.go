package docker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/toaweme/blink/core/config"
)

func Test_Docker_Name(t *testing.T) {
	if got := (Runtime{}).Name(); got != "docker" {
		t.Fatalf("Name() = %q, want %q", got, "docker")
	}
}

// Test_Docker_ResolveComposeFile asserts an unset `file` probes the conventional
// compose filenames in the same preference order the detector uses, so a
// hand-written `runtime: docker` on a compose.yaml project finds its stack
// instead of missing a non-existent docker-compose.yml.
func Test_Docker_ResolveComposeFile(t *testing.T) {
	tests := []struct {
		name    string
		present []string // filenames created in the work dir
		want    string   // basename resolveComposeFile should pick
	}{
		{
			name:    "compose.yaml is found",
			present: []string{"compose.yaml"},
			want:    "compose.yaml",
		},
		{
			name:    "compose.yaml wins over docker-compose.yml",
			present: []string{"docker-compose.yml", "compose.yaml"},
			want:    "compose.yaml",
		},
		{
			name:    "docker-compose.yaml is found",
			present: []string{"docker-compose.yaml"},
			want:    "docker-compose.yaml",
		},
		{
			name:    "no compose file falls back to the default",
			present: nil,
			want:    config.DefaultComposeFile,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, name := range tt.present {
				if err := os.WriteFile(filepath.Join(dir, name), []byte("services:\n"), 0o600); err != nil {
					t.Fatalf("write %s: %v", name, err)
				}
			}
			if got := resolveComposeFile(dir); got != tt.want {
				t.Fatalf("resolveComposeFile() = %q, want %q", got, tt.want)
			}
		})
	}
}

// Test_Docker_PrepareComposeFile asserts Prepare wires the probed compose file
// into the manager as an absolute path under the service dir when `file` is
// unset, and honors an explicit `file` unchanged.
func Test_Docker_PrepareComposeFile(t *testing.T) {
	tests := []struct {
		name     string
		file     string // explicit DockerConfig.File, "" for none
		present  string // compose filename created in the work dir
		wantBase string
	}{
		{
			name:     "probes compose.yaml when file unset",
			present:  "compose.yaml",
			wantBase: "compose.yaml",
		},
		{
			name:     "explicit file wins",
			file:     "custom.yml",
			present:  "compose.yaml",
			wantBase: "custom.yml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.present != "" {
				if err := os.WriteFile(filepath.Join(dir, tt.present), []byte("services:\n"), 0o600); err != nil {
					t.Fatalf("write %s: %v", tt.present, err)
				}
			}

			dc := &config.DockerConfig{}
			if tt.file != "" {
				dc.File = tt.file
			}
			svc := config.Service{Name: "docker", Dir: ".", Docker: dc}
			plan, err := Runtime{}.Prepare(config.Config{DirRoot: dir}, svc)
			if err != nil {
				t.Fatalf("Prepare: %v", err)
			}

			mgr, ok := plan.Manager.(*Manager)
			if !ok {
				t.Fatalf("plan.Manager = %T, want *Manager", plan.Manager)
			}
			wantAbs := filepath.Join(dir, tt.wantBase)
			if mgr.composeFile != wantAbs {
				t.Fatalf("composeFile = %q, want %q", mgr.composeFile, wantAbs)
			}
		})
	}
}
