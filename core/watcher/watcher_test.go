package watcher

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/toaweme/blink/core/config"
)

func TestExtSetNormalisesLeadingDotAndCase(t *testing.T) {
	set := extSet([]string{".Go", "yaml", ".JSON"})
	_, ok := set["go"]
	assert.True(t, ok)
	_, ok = set["yaml"]
	assert.True(t, ok)
	_, ok = set["json"]
	assert.True(t, ok)
	assert.Len(t, set, 3)
}

func TestExtSetEmpty(t *testing.T) {
	assert.Nil(t, extSet(nil))
	assert.Nil(t, extSet([]string{}))
}

func TestAnyMatch(t *testing.T) {
	g1, err := compileGlob("**/*.go")
	require.NoError(t, err)
	g2, err := compileGlob("vendor/**")
	require.NoError(t, err)

	globs := []interface{ Match(string) bool }{g1, g2}
	_ = globs

	assert.True(t, anyMatchHelper(g1, g2, "cmd/api/main.go"))
	assert.True(t, anyMatchHelper(g1, g2, "vendor/foo/bar.go"))
	assert.False(t, anyMatchHelper(g1, g2, "README.md"))
}

func anyMatchHelper(a, b interface{ Match(string) bool }, path string) bool {
	return a.Match(path) || b.Match(path)
}

func TestDedupe(t *testing.T) {
	got := dedupe([]string{"a", "b", "a", "c", "b"})
	assert.ElementsMatch(t, []string{"a", "b", "c"}, got)

	assert.Empty(t, dedupe(nil))
}

// Test_AddRecursive_NoDoubleCount verifies that overlapping watch roots (an
// Fs.Include directory nested under the implicit service-dir root) count each
// file and directory exactly once, rather than once per covering root.
func Test_AddRecursive_NoDoubleCount(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.go"), []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "b.go"), []byte("b"), 0o644))

	cfg := config.Config{DirRoot: root}
	svc := config.Service{
		Name:   "web",
		Dir:    ".",
		Reload: config.Reload{Reload: true},
		// "sub" is nested under the implicit DirRoot/. root, so without the
		// seen-set guard its files/dirs would be tallied twice.
		Fs: config.Fs{Include: []string{"sub"}},
	}

	w, err := New(cfg, svc)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, w.Start(ctx))

	files, dirs := w.Stats()
	assert.Equal(t, 2, files, "a.go + sub/b.go counted once each")
	// root + sub, counted once each despite "sub" being both an Include root
	// and a child of the service-dir root.
	assert.Equal(t, 2, dirs, "root + sub counted once each")
}
