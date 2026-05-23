package portkill

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/toaweme/blink/core/addon"
	"github.com/toaweme/blink/core/config"
)

func boolPtr(b bool) *bool { return &b }

func Test_Hook_Name(t *testing.T) {
	assert.Equal(t, "portkill", Hook{}.Name())
}

func Test_Hook_Phases(t *testing.T) {
	assert.Equal(t, []addon.Phase{addon.PhaseBeforeStart}, Hook{}.Phases())
}

func Test_Hook_RunNoopWhenPortsEmpty(t *testing.T) {
	// no ports declared: must not invoke kill logic, returns nil.
	err := Hook{}.Run(t.Context(), addon.PhaseBeforeStart, config.Config{ForceShutdown: boolPtr(true)}, config.Service{Name: "svc"})
	require.NoError(t, err)
}

func Test_Hook_RunNoopWhenForceShutdownDisabled(t *testing.T) {
	// per-service false wins over project-wide true: even with Ports set, the effective ForceShutdown is false, so Run short-circuits before Kill and returns nil.
	cfg := config.Config{ForceShutdown: boolPtr(true)}
	svc := config.Service{Name: "svc", Ports: []config.Port{config.LiteralPort(12345)}, ForceShutdown: boolPtr(false)}

	// verify the resolution itself
	assert.False(t, forceShutdownEnabled(cfg, svc))

	err := Hook{}.Run(t.Context(), addon.PhaseBeforeStart, cfg, svc)
	require.NoError(t, err)
}

func Test_ForceShutdownEnabled(t *testing.T) {
	tests := []struct {
		name string
		cfg  *bool
		svc  *bool
		want bool
	}{
		{"both nil defaults to true", nil, nil, true},
		{"per-service true wins", nil, boolPtr(true), true},
		{"per-service false wins over project true", boolPtr(true), boolPtr(false), false},
		{"per-service true wins over project false", boolPtr(false), boolPtr(true), true},
		{"project true with nil svc", boolPtr(true), nil, true},
		{"project false with nil svc", boolPtr(false), nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := forceShutdownEnabled(config.Config{ForceShutdown: tt.cfg}, config.Service{ForceShutdown: tt.svc})
			assert.Equal(t, tt.want, got)
		})
	}
}
