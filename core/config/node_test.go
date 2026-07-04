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

func Test_NodeDevCommandSelfReloads(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		want bool
	}{
		{"vite", "vite dev --port 3000", true},
		{"next", "next dev", true},
		{"nuxt", "nuxt dev", true},
		{"astro", "astro dev", true},
		{"remix vite", "remix vite:dev", true},
		{"webpack serve", "webpack serve --mode development", true},
		{"nodemon", "nodemon server.js", true},
		{"node --watch", "node --watch server.js", true},
		{"tsx watch", "tsx watch src/index.ts", true},
		{"ts-node-dev", "ts-node-dev --respawn src/main.ts", true},
		{"composed with vite", "tsr generate && vite dev", true},
		{"case insensitive", "NODE --WATCH main.js", true},
		{"plain node", "node server.js", false},
		{"build then run", "tsc && node dist/main.js", false},
		{"empty", "", false},
		{"serve static build", "serve -s dist", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NodeDevCommandSelfReloads(tt.cmd); got != tt.want {
				t.Fatalf("NodeDevCommandSelfReloads(%q) = %v, want %v", tt.cmd, got, tt.want)
			}
		})
	}
}

func Test_NodeDevScriptSelfReloads(t *testing.T) {
	tests := []struct {
		name    string
		pkgJSON string // "" writes no package.json
		script  string
		want    bool
	}{
		{"vite dev default script", `{"scripts":{"dev":"vite dev"}}`, "", true},
		{"plain node dev script", `{"scripts":{"dev":"node server.js"}}`, "", false},
		{"explicit non-dev script", `{"scripts":{"dev":"vite","serve":"node ."}}`, "serve", false},
		{"missing script", `{"scripts":{"build":"tsc"}}`, "", false},
		{"no package.json", "", "", false},
		{"malformed package.json", `{not json`, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.pkgJSON != "" {
				if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(tt.pkgJSON), 0o600); err != nil {
					t.Fatalf("write package.json: %v", err)
				}
			}
			if got := NodeDevScriptSelfReloads(dir, tt.script); got != tt.want {
				t.Fatalf("NodeDevScriptSelfReloads(%q, %q) = %v, want %v", dir, tt.script, got, tt.want)
			}
		})
	}
}
