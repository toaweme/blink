package loader

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/toaweme/blink/core/config"
)

func Test_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.Config
		wantErr string
	}{
		{
			name:    "empty services",
			cfg:     config.Config{},
			wantErr: "no services defined",
		},
		{
			name: "duplicate names",
			cfg: config.Config{Services: []config.Service{
				{Name: "a"}, {Name: "a"},
			}},
			wantErr: "duplicate service name: a",
		},
		{
			name: "missing dep",
			cfg: config.Config{Services: []config.Service{
				{Name: "a", Reload: config.Reload{ReloadOnService: []string{"ghost"}}},
			}},
			wantErr: `service "a" depends on unknown service "ghost"`,
		},
		{
			name: "self dep",
			cfg: config.Config{Services: []config.Service{
				{Name: "a", Reload: config.Reload{ReloadOnService: []string{"a"}}},
			}},
			wantErr: `service "a" depends on itself`,
		},
		{
			name: "unknown package manager",
			cfg: config.Config{Services: []config.Service{
				{Name: "web", Node: &config.NodeConfig{PackageManager: "deno"}},
			}},
			wantErr: `service "web" has unknown package_manager "deno"`,
		},
		{
			name: "known package manager",
			cfg: config.Config{Services: []config.Service{
				{Name: "web", Node: &config.NodeConfig{PackageManager: "pnpm"}},
			}},
		},
		{
			name: "empty package manager auto-detects",
			cfg: config.Config{Services: []config.Service{
				{Name: "web", Node: &config.NodeConfig{PackageManager: ""}},
			}},
		},
		{
			name: "valid",
			cfg: config.Config{Services: []config.Service{
				{Name: "a"},
				{Name: "b", Reload: config.Reload{ReloadOnService: []string{"a"}}},
			}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(tc.cfg)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func Test_Discover_FindsConfigInDir(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "blink.yaml"), []byte("services:\n  - name: x\n"), 0o644))

	got, err := Discover(dir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "blink.yaml"), got)
}

func Test_Discover_PrefersConfigNamesInOrder(t *testing.T) {
	// the canonical priority order: yml/yaml before toml before json. blink.yml
	// (first) is what `blink init` writes. Every name holds the same yaml bytes
	// here; this test only asserts discovery order, not decoding (see
	// Test_Load_DecodesByExtension for that).
	order := []string{"blink.yml", "blink.yaml", "blink.toml", "blink.json"}

	// walk the priority list, each case dropping the previous winner so the next
	// name in sequence must take over.
	for i := range order {
		present := order[i:]
		t.Run("falls through to "+order[i], func(t *testing.T) {
			dir := t.TempDir()
			for _, name := range present {
				require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("services:\n  - name: x\n"), 0o644))
			}
			got, err := Discover(dir)
			require.NoError(t, err)
			assert.Equal(t, filepath.Join(dir, order[i]), got,
				"with %v present, Discover must pick the highest-priority one", present)
		})
	}
}

func Test_Discover_WalksUp(t *testing.T) {
	dir := t.TempDir()
	deep := filepath.Join(dir, "a", "b", "c")
	require.NoError(t, os.MkdirAll(deep, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "blink.yaml"), []byte("services:\n  - name: x\n"), 0o644))

	got, err := Discover(deep)
	require.NoError(t, err, "Discover must walk up to parent directories")
	assert.Equal(t, filepath.Join(dir, "blink.yaml"), got)
}

func Test_Discover_ReturnsNotExistWhenMissing(t *testing.T) {
	dir := t.TempDir()

	_, err := Discover(dir)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func Test_Load_DecodesByExtension(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{"blink.yaml", "services:\n  - name: api\n"},
		{"blink.json", `{"services":[{"name":"api"}]}`},
		{"blink.toml", "[[services]]\nname = \"api\"\n"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, tc.name)
			require.NoError(t, os.WriteFile(path, []byte(tc.data), 0o644))

			cfg, abs, err := Load(dir, "")
			require.NoError(t, err)
			assert.Equal(t, path, abs)
			require.Len(t, cfg.Services, 1)
			assert.Equal(t, "api", cfg.Services[0].Name)
		})
	}
}

// Test_LoadRaw_KeepsFileAsAuthored guards the config round-trip used by
// `blink edit`: nothing the runtime loader derives (an absolute DirRoot, the
// resolved control/log/build dirs) may appear in a config read for rewriting,
// while values the file does set are handed back untouched.
func Test_LoadRaw_KeepsFileAsAuthored(t *testing.T) {
	tests := []struct {
		name        string
		data        string
		wantDirRoot string
		wantPaths   config.Paths
	}{
		{
			name: "omitted fields stay empty",
			data: "services:\n  - name: api\n",
		},
		{
			name:        "authored values are preserved",
			data:        "dir_root: ../app\npaths:\n    log_dir: tmp/logs\nservices:\n  - name: api\n",
			wantDirRoot: "../app",
			wantPaths:   config.Paths{LogDir: "tmp/logs"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "blink.yml")
			if err := os.WriteFile(path, []byte(tc.data), 0o644); err != nil {
				t.Fatalf("write config: %v", err)
			}

			cfg, abs, err := LoadRaw(dir, "")
			if err != nil {
				t.Fatalf("LoadRaw: %v", err)
			}
			if abs != path {
				t.Fatalf("path = %q, want %q", abs, path)
			}
			if cfg.DirRoot != tc.wantDirRoot {
				t.Fatalf("DirRoot = %q, want %q", cfg.DirRoot, tc.wantDirRoot)
			}
			if cfg.Paths != tc.wantPaths {
				t.Fatalf("Paths = %+v, want %+v", cfg.Paths, tc.wantPaths)
			}
			if cfg.ConfigPath != "" {
				t.Fatalf("ConfigPath = %q, want empty", cfg.ConfigPath)
			}
		})
	}
}

func Test_Load_ResolvesDirRootRelativeToConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "blink.yaml")
	require.NoError(t, os.WriteFile(path, []byte("services:\n  - name: x\n"), 0o644))

	cfg, abs, err := Load(dir, "")
	require.NoError(t, err)
	assert.Equal(t, path, abs)
	assert.Equal(t, dir, cfg.DirRoot, "DirRoot should default to config directory")
}
