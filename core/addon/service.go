package addon

import (
	"context"

	"github.com/toaweme/blink/core/config"
)

// Phase identifies a point in a single service's lifecycle. The supervisor
// invokes registered ServiceHooks at each phase. New phases can be added
// without breaking existing hooks, since hooks declare the phases they care
// about and the dispatcher only calls them at those.
type Phase string

// PhaseBeforeBuild and the other phases are the lifecycle points at which the
// supervisor dispatches registered ServiceHooks.
const (
	PhaseBeforeBuild Phase = "before-build"
	PhaseAfterBuild  Phase = "after-build"
	PhaseBeforeStart Phase = "before-start"
	PhaseAfterStart  Phase = "after-start"
	PhaseBeforeStop  Phase = "before-stop"
	PhaseAfterStop   Phase = "after-stop"
)

// ServiceHook is a cross-cutting concern that runs at specific lifecycle points
// for every service. Examples: portkill reclaims stale ports at
// PhaseBeforeStart; a secret injector decrypts secrets into the environment at
// PhaseBeforeStart.
//
// Unlike a Runtime (selected per-service via `runtime:`), a Hook applies to
// every service, or self-filters on the service it inspects. Unlike a
// Plan.Manager (which owns lifecycle), a Hook decorates it.
//
// Hook errors log at warn and do not abort the lifecycle; a hook may upgrade
// its failure mode by emitting a status event instead of returning an error.
type ServiceHook interface {
	Name() string
	// Phases lists every Phase this hook wants to be invoked at, letting the
	// dispatcher skip hooks that don't care about a given phase.
	Phases() []Phase
	// Run executes the hook at the given phase. A hook may be called for
	// multiple phases, switching on phase if behavior differs.
	Run(ctx context.Context, phase Phase, cfg config.Config, svc config.Service) error
}
