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

func Test_ExtSet_NormalisesLeadingDotAndCase(t *testing.T) {
	set := extSet([]string{".Go", "yaml", ".JSON"})
	_, ok := set["go"]
	assert.True(t, ok)
	_, ok = set["yaml"]
	assert.True(t, ok)
	_, ok = set["json"]
	assert.True(t, ok)
	assert.Len(t, set, 3)
}

func Test_ExtSet_Empty(t *testing.T) {
	assert.Nil(t, extSet(nil))
	assert.Nil(t, extSet([]string{}))
}

func Test_AnyMatch(t *testing.T) {
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

func Test_Dedupe(t *testing.T) {
	got := dedupe([]string{"a", "b", "a", "c", "b"})
	assert.ElementsMatch(t, []string{"a", "b", "c"}, got)

	assert.Empty(t, dedupe(nil))
}

// Test_MatchesChange_SetupTrigger verifies that a registered setup-trigger
// file (manifest or lockfile) is matched even when its extension is not in the
// watched set, while ordinary source files match without being flagged as
// setup triggers.
func Test_MatchesChange_SetupTrigger(t *testing.T) {
	cfg := config.Config{DirRoot: t.TempDir()}
	svc := config.Service{
		Name:   "web",
		Dir:    ".",
		Reload: config.Reload{Reload: true},
		Fs:     config.Fs{Extensions: []string{"js", "ts"}},
	}
	w, err := New(cfg, svc)
	require.NoError(t, err)
	w.SetSetupTriggers([]string{"package.json", "pnpm-lock.yaml"})

	root := cfg.DirRoot
	js := filepath.Join(root, "src", "app.js")
	pkg := filepath.Join(root, "package.json")
	lock := filepath.Join(root, "pnpm-lock.yaml") // .yaml is not a watched extension
	txt := filepath.Join(root, "notes.txt")

	assert.True(t, w.matchesChange(lock), "lockfile matches via setup trigger despite unwatched extension")
	assert.True(t, w.matchesChange(pkg), "manifest matches via setup trigger")
	assert.True(t, w.matchesChange(js), "watched source file matches normally")
	assert.False(t, w.matchesChange(txt), "unwatched, non-trigger file does not match")

	assert.True(t, w.anySetupTrigger([]string{js, pkg}), "a change set touching the manifest is a setup trigger")
	assert.False(t, w.anySetupTrigger([]string{js}), "a source-only change set is not a setup trigger")
}

// Test_ExtOK_CaseInsensitive verifies that a file's extension is matched
// case-insensitively against the configured set, so Main.GO, main.Go and
// main.go all match a configured "go" while an unlisted extension does not.
func Test_ExtOK_CaseInsensitive(t *testing.T) {
	cfg := config.Config{DirRoot: t.TempDir()}
	svc := config.Service{
		Name:   "web",
		Dir:    ".",
		Reload: config.Reload{Reload: true},
		Fs:     config.Fs{Extensions: []string{"go"}},
	}
	w, err := New(cfg, svc)
	require.NoError(t, err)

	tests := []struct {
		name string
		path string
		want bool
	}{
		{"uppercase extension", "Main.GO", true},
		{"mixed case extension", "main.Go", true},
		{"lowercase extension", "main.go", true},
		{"unlisted extension", "main.txt", false},
		{"unlisted uppercase extension", "main.TXT", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, w.extOK(tt.path))
		})
	}
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

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	require.NoError(t, w.Start(ctx))

	files, dirs := w.Stats()
	assert.Equal(t, 2, files, "a.go + sub/b.go counted once each")
	// root + sub, counted once each despite "sub" being both an Include root
	// and a child of the service-dir root.
	assert.Equal(t, 2, dirs, "root + sub counted once each")
}
