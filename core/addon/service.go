package addon

import (
	"context"

	"github.com/toaweme/blink/core/config"
)

// Phase identifies a well-defined point in a single service's lifecycle.
// The supervisor invokes registered ServiceHooks at each phase. New phases
// can be added without breaking existing hooks because hooks declare the
// phases they care about and the dispatcher only calls them at those.
type Phase string

const (
	PhaseBeforeBuild Phase = "before-build"
	PhaseAfterBuild  Phase = "after-build"
	PhaseBeforeStart Phase = "before-start"
	PhaseAfterStart  Phase = "after-start"
	PhaseBeforeStop  Phase = "before-stop"
	PhaseAfterStop   Phase = "after-stop"
)

// ServiceHook is a cross-cutting concern that wants to run at specific
// lifecycle points for every service. Examples:
//
//   - portkill: reclaim ports held by stale processes at PhaseBeforeStart.
//   - secret injector: at PhaseBeforeStart, decrypt and put secrets into
//     the service's environment.
//
// Hooks differ from Runtimes: a Runtime is selected per-service via
// `runtime:`, while a Hook applies to every service (or self-filters on
// the service it inspects). Hooks differ from Plan.Manager: a Manager
// owns lifecycle, a Hook decorates it.
//
// Errors from hooks log at warn by default and do not abort the
// lifecycle; individual hooks may upgrade their own failure mode by
// emitting a status event instead of returning an error.
type ServiceHook interface {
	Name() string
	// Phases lists every Phase this hook wants to be invoked at. The
	// dispatcher uses it to skip hooks that don't care about a given
	// phase, keeping the hot path cheap.
	Phases() []Phase
	// Run executes the hook at the given phase. The same hook may be
	// called for multiple phases over a service's lifetime; the
	// implementation switches on phase if it needs different behavior.
	Run(ctx context.Context, phase Phase, cfg config.Config, svc config.Service) error
}
