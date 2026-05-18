package loader

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/toaweme/blink/core/config"
)

func TestValidate(t *testing.T) {
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

func TestDiscoverFindsConfigInDir(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "blink.yaml"), []byte("services:\n  - name: x\n"), 0o644))

	got, err := Discover(dir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "blink.yaml"), got)
}

func TestDiscoverWalksUp(t *testing.T) {
	dir := t.TempDir()
	deep := filepath.Join(dir, "a", "b", "c")
	require.NoError(t, os.MkdirAll(deep, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "blink.yaml"), []byte("services:\n  - name: x\n"), 0o644))

	got, err := Discover(deep)
	require.NoError(t, err, "Discover must walk up to parent directories")
	assert.Equal(t, filepath.Join(dir, "blink.yaml"), got)
}

func TestDiscoverReturnsNotExistWhenMissing(t *testing.T) {
	dir := t.TempDir()

	_, err := Discover(dir)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestLoadResolvesDirRootRelativeToConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "blink.yaml")
	require.NoError(t, os.WriteFile(path, []byte("services:\n  - name: x\n"), 0o644))

	cfg, abs, err := Load(dir, "")
	require.NoError(t, err)
	assert.Equal(t, path, abs)
	assert.Equal(t, dir, cfg.DirRoot, "DirRoot should default to config directory")
}
