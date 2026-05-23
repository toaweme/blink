package supervisor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/toaweme/blink/core/addon"
	"github.com/toaweme/blink/core/config"
)

type stubRuntime struct{}

func (stubRuntime) Name() string { return "shell" }

func (stubRuntime) Prepare(_ config.Config, _ config.Service) (addon.Plan, error) {
	return addon.Plan{}, nil
}

func Test_TopoSort(t *testing.T) {
	tests := []struct {
		name     string
		services []config.Service
		want     []string
		wantErr  bool
	}{
		{
			name: "linear",
			services: []config.Service{
				{Name: "c", Reload: config.Reload{ReloadOnService: []string{"b"}}},
				{Name: "b", Reload: config.Reload{ReloadOnService: []string{"a"}}},
				{Name: "a"},
			},
			want: []string{"a", "b", "c"},
		},
		{
			name: "preserves input order when independent",
			services: []config.Service{
				{Name: "first"}, {Name: "second"}, {Name: "third"},
			},
			want: []string{"first", "second", "third"},
		},
		{
			name: "diamond",
			services: []config.Service{
				{Name: "root"},
				{Name: "left", Reload: config.Reload{ReloadOnService: []string{"root"}}},
				{Name: "right", Reload: config.Reload{ReloadOnService: []string{"root"}}},
				{Name: "top", Reload: config.Reload{ReloadOnService: []string{"left", "right"}}},
			},
			want: []string{"root", "left", "right", "top"},
		},
		{
			name: "cycle",
			services: []config.Service{
				{Name: "a", Reload: config.Reload{ReloadOnService: []string{"b"}}},
				{Name: "b", Reload: config.Reload{ReloadOnService: []string{"a"}}},
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := topoSort(tc.services)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func Test_New_BuildsDependentsMap(t *testing.T) {
	cfg := config.Config{Services: []config.Service{
		{Name: "a"},
		{Name: "b", Reload: config.Reload{ReloadOnService: []string{"a"}}},
		{Name: "c", Reload: config.Reload{ReloadOnService: []string{"a"}}},
	}}
	reg := addon.NewRegistry()
	reg.AddRuntime(stubRuntime{})
	s, err := New(cfg, reg)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"b", "c"}, s.services["a"].dependents)
	assert.Empty(t, s.services["b"].dependents)
}
