package config

import (
	"os"
	"path/filepath"
	"testing"
)

func Test_NodePackageManager(t *testing.T) {
	tests := []struct {
		name      string
		lockfiles []string
		want      string
	}{
		{"no lockfile falls back to npm", nil, "npm"},
		{"pnpm", []string{"pnpm-lock.yaml"}, "pnpm"},
		{"bun text lock", []string{"bun.lock"}, "bun"},
		{"bun binary lock", []string{"bun.lockb"}, "bun"},
		{"yarn", []string{"yarn.lock"}, "yarn"},
		{"npm", []string{"package-lock.json"}, "npm"},
		{"pnpm beats every other lockfile", []string{"pnpm-lock.yaml", "bun.lock", "yarn.lock", "package-lock.json"}, "pnpm"},
		{"bun beats yarn and npm", []string{"bun.lock", "yarn.lock", "package-lock.json"}, "bun"},
		{"yarn beats npm", []string{"yarn.lock", "package-lock.json"}, "yarn"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range tt.lockfiles {
				if err := os.WriteFile(filepath.Join(dir, f), nil, 0o600); err != nil {
					t.Fatalf("write %s: %v", f, err)
				}
			}
			if got := NodePackageManager(dir); got != tt.want {
				t.Fatalf("NodePackageManager() = %q, want %q", got, tt.want)
			}
		})
	}
}

func Test_NodeConfig_IsZero(t *testing.T) {
	tests := []struct {
		name string
		cfg  NodeConfig
		want bool
	}{
		{"empty is zero", NodeConfig{}, true},
		{"script set", NodeConfig{Script: "dev"}, false},
		{"package manager set", NodeConfig{PackageManager: "pnpm"}, false},
		{"install set", NodeConfig{Install: boolPtr(false)}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.IsZero(); got != tt.want {
				t.Fatalf("IsZero() = %v, want %v", got, tt.want)
			}
		})
	}
}

func boolPtr(b bool) *bool { return &b }
