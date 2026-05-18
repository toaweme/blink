package addon

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/toaweme/blink/core/config"
)

type fakeRT struct{ name string }

func (f fakeRT) Name() string                                            { return f.name }
func (f fakeRT) Prepare(_ config.Config, _ config.Service) (Plan, error) { return Plan{}, nil }

func Test_Registry_AddRuntime_And_Runtime(t *testing.T) {
	r := NewRegistry()
	r.AddRuntime(fakeRT{name: "shell"})
	r.AddRuntime(fakeRT{name: "fake"})

	got, err := r.Runtime("shell")
	require.NoError(t, err)
	assert.Equal(t, "shell", got.Name())

	got, err = r.Runtime("fake")
	require.NoError(t, err)
	assert.Equal(t, "fake", got.Name())
}

func Test_Registry_EmptyNameResolvesToShell(t *testing.T) {
	r := NewRegistry()
	r.AddRuntime(fakeRT{name: "shell"})

	got, err := r.Runtime("")
	require.NoError(t, err)
	assert.Equal(t, "shell", got.Name())
}

func Test_Registry_UnknownRuntime(t *testing.T) {
	r := NewRegistry()
	r.AddRuntime(fakeRT{name: "shell"})

	_, err := r.Runtime("nope")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), `"nope"`)
}

func Test_Registry_DuplicateRuntimePanics(t *testing.T) {
	r := NewRegistry()
	r.AddRuntime(fakeRT{name: "shell"})
	assert.Panics(t, func() {
		r.AddRuntime(fakeRT{name: "shell"})
	})
}
