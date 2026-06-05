package addon

import (
	"context"

	"github.com/toaweme/blink/core/config"
)

// Runtime is the per-ecosystem backend the supervisor consults when starting a
// service. It contributes a Plan with a defaults overlay (pure-default runtimes
// like "go") and/or a Manager that owns the lifecycle (full runtimes like
// "docker"). When Manager is nil the supervisor runs its standard build-run
// shell lifecycle.
type Runtime interface {
	// Name is the value users write under `runtime:` in blink.yaml.
	Name() string
	// Prepare inspects the user's service and returns a Plan. Called once per
	// service at supervisor startup.
	Prepare(cfg config.Config, svc config.Service) (Plan, error)
}
// Plan is the output of Runtime.Prepare. All fields are optional.
type Plan struct {
	// Defaults overlays missing fields on the user's Service. See MergeService.
	Defaults config.Service
	// ExtraWatches contributes additional recursive watch roots. Paths are
	// resolved against cfg.DirRoot by the watcher.
	ExtraWatches []string
	// SetupTriggers are base filenames (e.g. "package.json", "pnpm-lock.yaml")
	// whose change re-runs Commands.Setup in addition to the normal reload.
	// They are matched regardless of Fs.Extensions, so manifests and lockfiles
	// are observed even when their extension is not otherwise watched.
	SetupTriggers []string
	// Manager, if non-nil, takes over Start/Stop from the supervisor.
	Manager Manager
}

// Manager owns a service lifecycle for full runtimes. The supervisor calls
// Start once at boot and Stop on shutdown; restarts come through Restart.
// Status changes for the service and its children flow out through Events.
type Manager interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	// Restart is called when the supervisor receives a restart signal for
	// this service (file change, cascade, manual `r`).
	Restart(ctx context.Context) error
	// Events streams status transitions. Closed by Stop.
	Events() <-chan ManagerEvent
	// Logs returns a channel of lines for a named child, or nil if log
	// following isn't enabled for that child. The runtime is free to return
	// the same channel across calls.
	Logs(child string) <-chan LogLine
}

// ManagerEvent reports a state change for a service or one of its children.
type ManagerEvent struct {
	// Child is the optional sub-process name. Empty means the service itself.
	Child  string
	Status string // matches supervisor.Status values: running, exited, crashed, ...
	Err    error
	// Ports are the local TCP ports the service listens on, reported on a
	// service-level event (Child == "") by runtimes that discover them at
	// startup (e.g. docker compose published ports). Empty otherwise.
	Ports []int
}

// LogLine is a single line of captured output from a managed child process.
type LogLine struct {
	Child string
	Line  string
}
