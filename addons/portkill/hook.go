package portkill

import (
	"context"
	"errors"
	"fmt"

	"github.com/toaweme/blink/core/addon"
	"github.com/toaweme/blink/core/config"
	"github.com/toaweme/log"
)

// Hook is the ServiceHook adapter for portkill. Registered globally by
// the CLI, it runs at PhaseBeforeStart for every service - if the service
// declares Ports and has ForceShutdown effectively true, the hook
// reclaims those ports before the supervisor launches the runner.
//
// Failures are non-fatal (logged at warn) - matching the historical
// behavior where a missing lsof or a permission denied never blocks a
// service start.
type Hook struct{}

var _ addon.ServiceHook = Hook{}

// Name reports the hook's identifier.
func (Hook) Name() string { return "portkill" }

// Phases declares the lifecycle points this hook cares about.
func (Hook) Phases() []addon.Phase {
	return []addon.Phase{addon.PhaseBeforeStart}
}

// Run reclaims ports at PhaseBeforeStart when the service's effective
// ForceShutdown is true and Ports is non-empty.
func (Hook) Run(_ context.Context, _ addon.Phase, cfg config.Config, svc config.Service) error {
	if !forceShutdownEnabled(cfg, svc) || len(svc.Ports) == 0 {
		return nil
	}
	pids, err := Kill(svc.Ports)
	if err != nil {
		if errors.Is(err, ErrLsofMissing) {
			log.Warn("port reclaim skipped: lsof not installed", "service", svc.Name)
			return nil
		}
		return fmt.Errorf("failed to reclaim ports for %q: %w", svc.Name, err)
	}
	if len(pids) > 0 {
		log.Info("reclaimed ports", "service", svc.Name, "ports", svc.Ports, "killed", pids)
	}
	return nil
}

// forceShutdownEnabled resolves the effective ForceShutdown for a service.
// Per-service value wins; falls back to Config.ForceShutdown; defaults to
// true (the whole point of this feature is to keep stale children from
// blocking a fresh bind).
func forceShutdownEnabled(cfg config.Config, svc config.Service) bool {
	if svc.ForceShutdown != nil {
		return *svc.ForceShutdown
	}
	if cfg.ForceShutdown != nil {
		return *cfg.ForceShutdown
	}
	return true
}
