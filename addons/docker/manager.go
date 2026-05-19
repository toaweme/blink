package docker

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"github.com/toaweme/blink/core/addon"
	"github.com/toaweme/log"
)

// Manager implements addon.Manager for a docker compose stack.
type Manager struct {
	project     string
	composeFile string
	workDir     string
	services    []string // compose service subset (empty = all)
	logFilter   []string // services whose logs to stream (empty = every running container)
	wait        bool
	stopOnExit  bool

	events chan addon.ManagerEvent
	// logCh is the single multiplexed stream of every followed container's
	// output. Each line carries its container in LogLine.Child, so the TUI can
	// filter by container without the supervisor having to know the set up
	// front. The supervisor consumes it via Logs("").
	logCh chan addon.LogLine

	mu      sync.Mutex
	cancel  context.CancelFunc // cancels event + log streamers
	started bool

	// startedByUs is the set of compose services that were not running before
	// blink invoked `up -d`. Populated in Start when stopOnExit is true so that
	// Stop only touches the containers blink itself brought up.
	startedByUs map[string]bool
}

var _ addon.Manager = (*Manager)(nil)

type managerOpts struct {
	Project     string
	ComposeFile string
	WorkDir     string
	Services    []string
	LogFilter   []string
	Wait        bool
	StopOnExit  bool
}

func newManager(opts managerOpts) *Manager {
	return &Manager{
		project:     opts.Project,
		composeFile: opts.ComposeFile,
		workDir:     opts.WorkDir,
		services:    opts.Services,
		logFilter:   opts.LogFilter,
		wait:        opts.Wait,
		stopOnExit:  opts.StopOnExit,
		events:      make(chan addon.ManagerEvent, 32),
		logCh:       make(chan addon.LogLine, 256),
	}
}

func (m *Manager) Events() <-chan addon.ManagerEvent { return m.events }

// Logs returns the aggregate container-log stream for the whole stack (child
// == ""). Per-child lookups return nil: the runtime multiplexes every
// container onto the one channel and tags each line with its container name.
func (m *Manager) Logs(child string) <-chan addon.LogLine {
	if child == "" {
		return m.logCh
	}
	return nil
}

func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.started {
		return nil
	}

	// Snapshot which services were already running so Stop knows what NOT to
	// touch on exit. Only needed when we actually plan to stop things.
	preRunning := map[string]bool{}
	if m.stopOnExit {
		if rows, err := m.composeRows(ctx); err == nil {
			for _, row := range rows {
				if row.State == "running" {
					preRunning[row.Service] = true
				}
			}
		}
	}

	args := []string{"compose", "-p", m.project, "-f", m.composeFile, "up", "-d"}
	if m.wait {
		args = append(args, "--wait")
	}
	args = append(args, m.services...)

	log.Info("docker compose up", "project", m.project, "file", m.composeFile, "services", m.services)
	if err := m.runComposeBlocking(ctx, args...); err != nil {
		return fmt.Errorf("failed to run docker compose up: %w", err)
	}

	if m.stopOnExit {
		if rows, err := m.composeRows(ctx); err == nil {
			m.startedByUs = map[string]bool{}
			for _, row := range rows {
				if !preRunning[row.Service] {
					m.startedByUs[row.Service] = true
				}
			}
		}
	}

	if err := m.seedStatus(ctx); err != nil {
		log.Warn("docker: ps snapshot failed", "error", err, "project", m.project)
	}

	// `--wait` only waits for explicit healthchecks. Containers without one
	// (stock mysql/postgres/etc.) are "running" before the daemon inside
	// answers. Probe published TCP ports until they accept so dependents
	// don't start against a not-yet-listening backend.
	if m.wait {
		if err := m.waitForPublishedPorts(ctx); err != nil {
			log.Warn("docker: port readiness probe failed", "error", err)
		}
	}

	streamCtx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	go m.runEventStream(streamCtx)
	for _, name := range m.followSet(ctx) {
		go m.runLogStream(streamCtx, name)
	}

	m.emit(addon.ManagerEvent{Status: "running"})
	m.started = true
	return nil
}

// followSet resolves which containers to tail. By default every service in the
// running stack is followed; an explicit DockerConfig.Logs narrows that to the
// listed subset. Falls back to the configured service subset when `ps` can't
// be read (e.g. compose too old to emit json).
func (m *Manager) followSet(ctx context.Context) []string {
	if len(m.logFilter) > 0 {
		return m.logFilter
	}
	rows, err := m.composeRows(ctx)
	if err != nil {
		log.Warn("docker: could not list containers to follow logs", "error", err, "project", m.project)
		return m.services
	}
	seen := make(map[string]bool, len(rows))
	var names []string
	for _, row := range rows {
		if row.Service == "" || seen[row.Service] {
			continue
		}
		seen[row.Service] = true
		names = append(names, row.Service)
	}
	return names
}

func (m *Manager) Stop(ctx context.Context) error {
	m.mu.Lock()
	cancel := m.cancel
	m.cancel = nil
	m.started = false
	startedByUs := m.startedByUs
	m.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	// Default behavior: leave containers running so the next `blink run`
	// reuses warm databases. Only stop when the user opted in, and even then
	// only the services we ourselves started - never pre-existing containers.
	if m.stopOnExit && len(startedByUs) > 0 {
		ours := make([]string, 0, len(startedByUs))
		for name := range startedByUs {
			ours = append(ours, name)
		}
		args := append([]string{"compose", "-p", m.project, "-f", m.composeFile, "stop"}, ours...)
		log.Info("docker compose stop", "project", m.project, "services", ours)
		if err := m.runComposeBlocking(ctx, args...); err != nil {
			log.Warn("docker compose stop failed", "error", err)
		}
	}

	m.emit(addon.ManagerEvent{Status: "stopped"})
	// closing events here would race with late status emits; leave it open -
	// the supervisor stops listening when its context cancels.
	return nil
}

func (m *Manager) Restart(ctx context.Context) error {
	args := []string{"compose", "-p", m.project, "-f", m.composeFile, "restart"}
	args = append(args, m.services...)
	log.Info("docker compose restart", "project", m.project)
	return m.runComposeBlocking(ctx, args...)
}

func (m *Manager) emit(ev addon.ManagerEvent) {
	select {
	case m.events <- ev:
	default:
	}
}

func (m *Manager) emitLog(child, line string) {
	select {
	case m.logCh <- addon.LogLine{Child: child, Line: line}:
	default:
		// Drop on slow consumer; preserves liveness in TUI.
	}
}

// runComposeBlocking runs `docker <args>` to completion. On success the
// combined output is dropped (compose's chatter is noise once it's
// working); on failure we surface a clipped tail in the error so the
// user actually sees *why* compose failed instead of bare "exit status
// 1", and the full output is also forwarded to the manager's log stream
// so it lands in the service's log tab.
func (m *Manager) runComposeBlocking(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = m.workDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(out))
		log.Debug("docker output", "output", text, "args", args)
		// fan the captured output out to the service tab so users see compose
		// errors inline. Best-effort - dropped on slow consumer.
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			m.emitLog("", "docker: "+line)
		}
		if text == "" {
			return err
		}
		// Cap the inline error at ~600 chars so a giant compose stack
		// trace doesn't drown the status bar; the full text already
		// reached the log tab above.
		if len(text) > 600 {
			text = "…" + text[len(text)-600:]
		}
		return fmt.Errorf("failed to run docker compose %v: %s: %w", args, text, err)
	}
	return nil
}
