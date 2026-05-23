package golang

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/toaweme/blink/core/config"
)

func Test_ReadWorkUses(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "go.work")
	content := "" +
		"go 1.22\n" +
		"\n" +
		"use (\n" +
		"\t.\n" +
		"\t../../awee-ai/cli\n" +
		"\t../log // local toaweme/log\n" +
		")\n" +
		"\n" +
		"use ./flat\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	uses, err := readWorkUses(path)
	require.NoError(t, err)
	assert.Equal(t, []string{".", "../../awee-ai/cli", "../log", "./flat"}, uses)
}

func Test_Prepare_RequiresPackage(t *testing.T) {
	_, err := Runtime{}.Prepare(
		config.Config{},
		config.Service{Name: "x", Go: &config.GoConfig{}},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "go.package is required")

	_, err = Runtime{}.Prepare(
		config.Config{},
		config.Service{Name: "x"},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires a `go:` block")
}

func Test_Prepare_SynthesizesCommands(t *testing.T) {
	plan, err := Runtime{}.Prepare(
		config.Config{DirRoot: t.TempDir()},
		config.Service{
			Name: "api.schema",
			Go: &config.GoConfig{
				Package: "./cmd/v2/schema",
				Args:    []string{"run", "--verbosity=2"},
			},
		},
	)
	require.NoError(t, err)
	require.NotNil(t, plan.Defaults.Commands.Build)
	require.NotNil(t, plan.Defaults.Commands.Run)
	assert.Equal(t, "go build -o ./build/api.schema ./cmd/v2/schema", plan.Defaults.Commands.Build.Command)
	assert.Equal(t, "./build/api.schema run --verbosity=2", plan.Defaults.Commands.Run.Command)
	assert.True(t, plan.Defaults.Commands.Run.Service)
	assert.Equal(t, []string{"go", "mod", "sum"}, plan.Defaults.Fs.Extensions)
}

func Test_Prepare_WorkspaceWatches(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "api"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "shared"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "go.work"),
		[]byte("go 1.22\nuse (\n\t./api\n\t./shared\n)\n"),
		0o644,
	))

	plan, err := Runtime{}.Prepare(
		config.Config{DirRoot: dir},
		config.Service{
			Name: "api",
			Dir:  "api",
			Go:   &config.GoConfig{Package: "."},
		},
	)
	require.NoError(t, err)

	wantShared := filepath.Join(dir, "shared")
	assert.Contains(t, plan.ExtraWatches, wantShared)
	// service's own dir must not be in ExtraWatches (already watched via Dir).
	wantAPI := filepath.Join(dir, "api")
	assert.NotContains(t, plan.ExtraWatches, wantAPI)
}
