package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/toaweme/blink/core/addon"
	"github.com/toaweme/blink/core/config"
	"github.com/toaweme/blink/core/config/detect"
	"github.com/toaweme/blink/core/portprobe"
	"github.com/toaweme/blink/core/supervisor"
)

// probeTimeout is the overall backstop for a service to build and reach running
// before giving up. Generous because go/air services compile first; most probes
// finish far sooner (see probeRunGrace).
const probeTimeout = 30 * time.Second

// probeRunGrace is how long to keep watching for a port once the service is
// running. A server binds within a moment of starting, so a service running this
// long without a listener almost certainly doesn't listen; stopping early keeps
// probing snappy for workers and other non-listening services.
const probeRunGrace = 3 * time.Second

// probeSettle is the extra wait after the first port appears, so a service that
// opens several ports in quick succession has them all captured.
const probeSettle = 600 * time.Millisecond

// probePoll is how often the service's process group is re-checked while waiting.
const probePoll = 150 * time.Millisecond

// runtimeProbe spins a single service up via a throwaway supervisor and returns
// the TCP ports it bound, read from the listening sockets owned by the service's
// process group. Unlike detect.SniffPorts (which guesses from .env), this
// observes what the process actually listened on, for any runtime or layout.
// Per-group attribution lets several services be probed concurrently without
// stealing each other's ports.
//
// A discovered port is mapped back to its env-var name when the service's .env
// already names it, so init can write the reference instead of the literal. The
// service is always stopped before returning, even on error. Docker is rejected
// up front: bringing a compose stack up to read ports is too heavy, and compose
// already declares its ports.
func runtimeProbe(ctx context.Context, reg *addon.Registry, root string, svc config.Service) ([]config.Port, error) {
	if svc.Runtime == "docker" {
		return nil, errors.New("port probing is not supported for docker services: ports come from the compose file")
	}

	// probe a lone copy: drop file-watch and cross-service reload deps so the
	// single-service supervisor doesn't reject a dep on an absent sibling, and so
	// no watcher spins up for a process killed in seconds.
	probed := svc
	probed.Reload = config.Reload{}
	cfg := config.Config{DirRoot: root, Services: []config.Service{probed}}
	cfg.Paths.Resolve(root)

	sup, err := supervisor.New(cfg, reg)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare service %q for probing: %w", svc.Name, err)
	}
	if err := sup.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start service %q for probing: %w", svc.Name, err)
	}
	defer func() {
		// keep ctx's values but drop its cancellation: the service must still be
		// stopped even when the parent ctx was already canceled (e.g. init returning).
		stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		_ = sup.Stop(stopCtx)
	}()

	ports, err := waitForGroupPorts(ctx, sup, svc.Name)
	if err != nil {
		return nil, err
	}
	return portsToConfig(root, svc, ports), nil
}

// waitForGroupPorts polls the service's process group until it owns a listening
// port, or it's clear none is coming. Each service runs in its own process
// group, so the runner's pid is the group id. It returns early when:
//   - a port appears (after a brief probeSettle to catch sibling ports);
//   - the service crashed, exited, or was stopped (no listener to find);
//   - the service has run for probeRunGrace without binding a port (not a listening service, e.g. a worker).
//
// probeTimeout is only the overall backstop for a service stuck building.
func waitForGroupPorts(ctx context.Context, sup *supervisor.Supervisor, name string) ([]int, error) {
	deadline := time.Now().Add(probeTimeout)
	ticker := time.NewTicker(probePoll)
	defer ticker.Stop()
	var runningSince time.Time
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			switch sup.Status(name) {
			case supervisor.StatusCrashed, supervisor.StatusExited, supervisor.StatusStopped:
				// build failed, one-shot finished, or process gone: nothing listens.
				return nil, nil
			case supervisor.StatusRunning:
				if runningSince.IsZero() {
					runningSince = time.Now()
				}
				if pgid := pgidOf(sup, name); pgid > 0 {
					ports, err := portprobe.ListenPorts(pgid)
					if err != nil {
						return nil, fmt.Errorf("failed to read listening ports: %w", err)
					}
					if len(ports) > 0 {
						select {
						case <-ctx.Done():
							return nil, ctx.Err()
						case <-time.After(probeSettle):
						}
						if settled, err := portprobe.ListenPorts(pgid); err == nil && len(settled) > 0 {
							return settled, nil
						}
						return ports, nil
					}
				}
				if time.Since(runningSince) > probeRunGrace {
					return nil, nil // running a while, bound nothing
				}
			default:
				// pending / building / restarting: keep polling until running,
				// terminal, or the overall deadline below.
			}
			if time.Now().After(deadline) {
				return nil, nil
			}
		}
	}
}

// pgidOf returns the process-group id of the service's running process (its
// runner pid), or 0 if there's no host process yet.
func pgidOf(sup *supervisor.Supervisor, name string) int {
	r := sup.Runner(name)
	if r == nil {
		return 0
	}
	return r.Pid()
}

// portsToConfig turns discovered literal ports into config.Ports, substituting
// an env-var name for any port the service's .env already names.
func portsToConfig(root string, svc config.Service, ports []int) []config.Port {
	out := make([]config.Port, 0, len(ports))
	for _, p := range ports {
		if key, ok := detect.EnvKeyForPort(root, svc, p); ok {
			out = append(out, config.EnvPort(key))
			continue
		}
		out = append(out, config.LiteralPort(p))
	}
	return out
}
