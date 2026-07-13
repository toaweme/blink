// Package supervisor orchestrates the configured services: it starts them in
// dependency order, restarts them (and their dependents) on file changes, and
// fans status transitions and captured output out through a single Hub.
package supervisor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/toaweme/log"

	"github.com/toaweme/blink/core/addon"
	"github.com/toaweme/blink/core/config"
	"github.com/toaweme/blink/core/exec"
	"github.com/toaweme/blink/core/output"
	"github.com/toaweme/blink/core/protocol"
	"github.com/toaweme/blink/core/watcher"
)

// Status is the lifecycle state of a single service.
type Status string

const (
	// StatusPending is the initial state before a service has started.
	StatusPending Status = "pending"
	// StatusBuilding means a build step is running before the service starts.
	StatusBuilding Status = "building"

	// StatusInstalling means the service's one-time Setup commands are running.
	StatusInstalling Status = "installing"
	// StatusRunning means the service process is up.
	StatusRunning Status = "running"
	// StatusRestarting means the service is being restarted.
	StatusRestarting Status = "restarting"
	// StatusExited means the process exited cleanly.
	StatusExited Status = "exited"
	// StatusCrashed means the process exited with a non-zero status.
	StatusCrashed Status = "crashed"
	// StatusStopped means the service was stopped by the supervisor.
	StatusStopped Status = "stopped"
)

// ErrDependencyFailed is returned by waitForDeps when a dependency reaches a
// terminal failure state before becoming ready. The dependent gives up instead
// of waiting forever, so a crashed dependency surfaces as a clear diagnostic
// rather than a silent hang.
var ErrDependencyFailed = errors.New("dependency failed to start")

// isTerminalFailure reports whether status is a terminal state a dependency can
// never recover from on its own, so a dependent waiting on it must give up
// rather than block forever. StatusStopped is deliberately excluded: it happens
// during shutdown, where the context cancel already unblocks the wait.
func isTerminalFailure(status Status) bool {
	return status == StatusCrashed
}

// Supervisor orchestrates the configured services. Status transitions and
// captured log lines flow out through a single Hub; consumers (TUI, plain UI,
// headless log writer, remote mirror) subscribe for their own independent
// channels of protocol.StatusEvent and protocol.LogLine.
//
// Cross-cutting lifecycle work (port reclaim, future secret injection) goes
// through the optional hooks registry. A nil registry means no hooks, and the
// supervisor simply skips the dispatch points.
type Supervisor struct {
	cfg      config.Config
	registry *addon.Registry

	services map[string]*serviceState
	order    []string

	hub *output.Hub

	ctx    context.Context //nolint:containedctx // stored run context, derived in Start and canceled in Stop, drives every service goroutine's lifecycle.
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// stdinEnabled gives every shell-lifecycle service its own stdin pipe. Off
	// by default (services inherit the parent's stdin); enabled by `blink run`
	// when the control socket is configured so the "send" op has a destination.
	stdinEnabled bool
}

// EnableStdin toggles stdin pipes on the runners managed by this supervisor.
// Must be called before Start. No-op once services are running.
func (s *Supervisor) EnableStdin(on bool) { s.stdinEnabled = on }

// WatchStats returns the aggregate file and directory counts across every
// service's watcher. Both zero when no watchers are running.
func (s *Supervisor) WatchStats() (files, dirs int) {
	for _, st := range s.services {
		st.mu.Lock()
		w := st.watcher
		st.mu.Unlock()
		if w == nil {
			continue
		}
		f, d := w.Stats()
		files += f
		dirs += d
	}
	return files, dirs
}

// WatchStat is one service's watcher counts.
type WatchStat struct {
	Files int
	Dirs  int
}

// WatchStatsByService returns the file and dir counts keyed by service name.
// Services without a watcher are omitted.
func (s *Supervisor) WatchStatsByService() map[string]WatchStat {
	out := make(map[string]WatchStat, len(s.services))
	for name, st := range s.services {
		st.mu.Lock()
		w := st.watcher
		st.mu.Unlock()
		if w == nil {
			continue
		}
		f, d := w.Stats()
		out[name] = WatchStat{Files: f, Dirs: d}
	}
	return out
}

type serviceState struct {
	svc        config.Service
	plan       addon.Plan
	dependents []string

	restart chan struct{}
	done    chan struct{}

	// setupPending is set by the watcher path when a setup-trigger file changes,
	// so the next lifecycle re-runs Commands.Setup even though it is not a boot.
	setupPending atomic.Bool

	mu      sync.Mutex
	runner  *exec.Runner
	status  Status
	watcher *watcher.Watcher
}

// New builds a Supervisor for cfg, resolving runtimes from reg and merging
// defaults. Returns an error on cyclic or invalid deps or unknown runtimes. The
// caller owns the registry and must register every runtime to be looked up,
// including "shell".
func New(cfg config.Config, reg *addon.Registry) (*Supervisor, error) {
	s := &Supervisor{
		cfg:      cfg,
		registry: reg,
		services: make(map[string]*serviceState, len(cfg.Services)),
		hub:      output.NewHub(),
	}

	// resolve runtimes and merge defaults before other validation, so the rest
	// of setup sees the effective service.
	merged := make([]config.Service, len(cfg.Services))
	plans := make([]addon.Plan, len(cfg.Services))
	for i, svc := range cfg.Services {
		rt, err := reg.Runtime(svc.Runtime)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve runtime for service %q: %w", svc.Name, err)
		}
		plan, err := rt.Prepare(cfg, svc)
		if err != nil {
			return nil, err
		}
		merged[i] = addon.MergeService(svc, plan.Defaults)
		plans[i] = plan
	}

	for i, svc := range merged {
		s.services[svc.Name] = &serviceState{
			svc:     svc,
			plan:    plans[i],
			restart: make(chan struct{}, 1),
			done:    make(chan struct{}),
			status:  StatusPending,
		}
	}

	for _, svc := range merged {
		for _, dep := range svc.Reload.ReloadOnService {
			if _, ok := s.services[dep]; !ok {
				return nil, fmt.Errorf("service %q depends on unknown service %q", svc.Name, dep)
			}
			s.services[dep].dependents = append(s.services[dep].dependents, svc.Name)
		}
	}

	order, err := topoSort(merged)
	if err != nil {
		return nil, err
	}
	s.order = order

	return s, nil
}

// Subscribe registers a consumer on the supervisor's event bus. The returned
// cancel func unregisters the subscription; channels close on cancel or when
// the supervisor stops.
func (s *Supervisor) Subscribe() (output.Subscription, func()) { return s.hub.Subscribe() }

// Order returns the service names in dependency-resolved start order.
func (s *Supervisor) Order() []string { return append([]string(nil), s.order...) }

// InsertBlank publishes an empty log line for a service onto the Hub so every
// subscriber (TUI buffer, per-service .log file, remote mirror) gets the same
// visual spacer. Used by the TUI's enter key to break up output.
func (s *Supervisor) InsertBlank(service string) {
	s.hub.PublishLog(protocol.LogLine{Service: service, Line: ""})
}

// Runner returns the current process runner for a service, or nil (including
// runtime-managed services like docker where there is no host process).
func (s *Supervisor) Runner(name string) *exec.Runner {
	st, ok := s.services[name]
	if !ok {
		return nil
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.runner
}

// Status returns the current lifecycle Status of a service, or StatusPending
// if the name is unknown.
func (s *Supervisor) Status(name string) Status {
	st, ok := s.services[name]
	if !ok {
		return StatusPending
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.status
}

// Start launches every configured service and its file watchers, deriving an
// internal context from ctx that Stop later cancels.
func (s *Supervisor) Start(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)

	for _, name := range s.order {
		st := s.services[name]
		s.wg.Add(1)
		if st.plan.Manager != nil {
			go s.runManaged(st)
		} else {
			go s.run(st)
		}
	}

	for _, name := range s.order {
		st := s.services[name]
		if !st.svc.Reload.Reload && len(st.svc.Reload.ReloadOnDelete) == 0 {
			// no reload configured, so this service gets no file watcher. skip
			// watcher.New (which would only error for this case) and give the user
			// a friendly hint at info level rather than staying silent.
			log.Info("no reload configured, service will not restart on file changes (add reload.reload to enable)", "service", name)
			continue
		}
		w, err := watcher.New(s.cfg, st.svc, st.plan.ExtraWatches...)
		if err != nil {
			log.Warn("supervisor: watcher init failed", "service", name, "error", err)
			continue
		}
		w.SetSetupTriggers(st.plan.SetupTriggers)
		if err := w.Start(s.ctx); err != nil { //nolint:contextcheck // s.ctx is derived from Start's ctx
			log.Warn("supervisor: watcher start failed", "service", name, "error", err)
			continue
		}
		st.mu.Lock()
		st.watcher = w
		st.mu.Unlock()
		s.wg.Add(1)
		go s.forwardWatcher(name, w)
	}

	return nil
}

// Stop cancels the run context, stops every service in reverse start order,
// waits for all goroutines to drain, and closes the Hub.
func (s *Supervisor) Stop(ctx context.Context) error {
	if s.cancel != nil {
		s.cancel()
	}

	for i := len(s.order) - 1; i >= 0; i-- {
		st := s.services[s.order[i]]
		if st.plan.Manager != nil {
			_ = st.plan.Manager.Stop(ctx)
		} else {
			s.stopService(st)
			s.runCleanup(st)
		}
	}

	s.wg.Wait()
	s.hub.Close()
	return nil
}

// Restart signals a single service to restart, returning an error if the name
// is unknown.
func (s *Supervisor) Restart(name string) error {
	st, ok := s.services[name]
	if !ok {
		return fmt.Errorf("unknown service %q", name)
	}
	select {
	case st.restart <- struct{}{}:
	default:
	}
	return nil
}

// RestartAll signals every configured service to restart.
func (s *Supervisor) RestartAll() {
	for _, name := range s.order {
		_ = s.Restart(name)
	}
}

// run owns the lifecycle of a shell-runtime service.
func (s *Supervisor) run(st *serviceState) {
	defer s.wg.Done()
	defer close(st.done)

	if err := s.waitForDeps(st); err != nil {
		s.setStatusErr(st, StatusCrashed, err)
		return
	}
	s.lifecycle(st, true)

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-st.restart:
			s.publishStatus(st.svc.Name, "", StatusRestarting, nil)
			s.stopRunner(st)
			s.lifecycle(st, false)
			s.cascadeRestart(st)
		}
	}
}

// runManaged delegates lifecycle to an addon.Manager (e.g. docker compose),
// forwarding its events into the supervisor's stream so the UI sees per-child
// status changes.
func (s *Supervisor) runManaged(st *serviceState) {
	defer s.wg.Done()
	defer close(st.done)

	if err := s.waitForDeps(st); err != nil {
		s.setStatusErr(st, StatusCrashed, err)
		return
	}

	mgr := st.plan.Manager

	s.wg.Add(1)
	go s.forwardManagerEvents(st)
	// service-level output (Child==""). The docker runtime multiplexes every
	// container onto this stream, tagging each line with LogLine.Child, so
	// per-container logs flow without the supervisor knowing the container set
	// up front.
	if ch := mgr.Logs(""); ch != nil {
		s.wg.Add(1)
		go s.forwardManagerLogs(st.svc.Name, ch)
	}
	for _, child := range collectChildren(st.svc) {
		if ch := mgr.Logs(child); ch != nil {
			s.wg.Add(1)
			go s.forwardManagerLogs(st.svc.Name, ch)
		}
	}

	s.setStatus(st, StatusBuilding)
	if err := mgr.Start(s.ctx); err != nil {
		s.setStatusErr(st, StatusCrashed, err)
		return
	}
	s.setStatus(st, StatusRunning)

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-st.restart:
			s.publishStatus(st.svc.Name, "", StatusRestarting, nil)
			if err := mgr.Restart(s.ctx); err != nil {
				s.setStatusErr(st, StatusCrashed, err)
				continue
			}
			s.setStatus(st, StatusRunning)
			s.cascadeRestart(st)
		}
	}
}

// collectChildren returns named per-child log channels to pull in addition to
// the aggregate Logs("") stream. Docker streams everything through Logs(""), so
// this stays empty; it is the seam for a runtime exposing one channel per named
// child.
func collectChildren(svc config.Service) []string {
	return nil
}

func (s *Supervisor) forwardManagerEvents(st *serviceState) {
	defer s.wg.Done()
	mgr := st.plan.Manager
	for {
		select {
		case <-s.ctx.Done():
			return
		case ev, ok := <-mgr.Events():
			if !ok {
				return
			}
			s.publishStatus(st.svc.Name, ev.Child, Status(ev.Status), ev.Err, ev.Ports...)
		}
	}
}

func (s *Supervisor) forwardManagerLogs(service string, ch <-chan addon.LogLine) {
	defer s.wg.Done()
	for {
		select {
		case <-s.ctx.Done():
			return
		case line, ok := <-ch:
			if !ok {
				return
			}
			s.publishLog(service, line.Child, line.Line)
		}
	}
}

// lifecycle drives one boot-or-restart of a service: optional setup, optional
// build, then the run command. boot is true only for the initial start; setup
// also runs when a setup-trigger file changed since the last lifecycle.
func (s *Supervisor) lifecycle(st *serviceState, boot bool) {
	if s.ctx.Err() != nil {
		return
	}
	if runSetup := boot || st.setupPending.Swap(false); runSetup && len(st.svc.Commands.Setup) > 0 {
		s.setStatus(st, StatusInstalling)
		if err := s.runChain(st, st.svc.Commands.Setup); err != nil {
			s.setStatusErr(st, StatusCrashed, err)
			log.Error("setup failed", "service", st.svc.Name, "error", err)
			return
		}
		if s.ctx.Err() != nil {
			return
		}
	}
	if st.svc.Commands.Build != nil {
		s.setStatus(st, StatusBuilding)
		if err := s.runCommandChain(st, *st.svc.Commands.Build); err != nil {
			s.setStatusErr(st, StatusCrashed, err)
			log.Error("build failed", "service", st.svc.Name, "error", err)
			return
		}
	}
	if s.ctx.Err() != nil {
		return
	}

	if st.svc.Commands.Run == nil {
		s.setStatus(st, StatusRunning)
		return
	}

	if err := s.runChain(st, st.svc.Commands.Run.Before); err != nil {
		s.setStatusErr(st, StatusCrashed, err)
		return
	}
	if s.ctx.Err() != nil {
		return
	}

	main := *st.svc.Commands.Run
	if main.Command == "" {
		_ = s.runChain(st, st.svc.Commands.Run.After)
		s.setStatus(st, StatusExited)
		return
	}

	s.dispatchHooks(addon.PhaseBeforeStart, st.svc)

	runner := exec.NewRunner(s.execConfig(st.svc, main))
	runner.SetTee(s.tee(st.svc.Name))
	st.mu.Lock()
	st.runner = runner
	st.mu.Unlock()

	s.setStatus(st, StatusRunning)
	go func() {
		err := runner.Run()
		// once Run returns, drop our runner slot so the next restart's stopRunner
		// cannot signal this dead (and possibly pid-reused) process group. If a
		// restart or stop already swapped in a different runner, that run now owns
		// the service, so this stale goroutine must not touch its status.
		st.mu.Lock()
		superseded := st.runner != runner
		if !superseded {
			st.runner = nil
		}
		st.mu.Unlock()
		if superseded {
			return
		}
		if err != nil {
			s.setStatusErr(st, StatusCrashed, err)
			log.Error("process exited unexpectedly", "service", st.svc.Name, "error", err)
			return
		}
		if main.Service {
			s.setStatus(st, StatusExited)
			return
		}
		_ = s.runChain(st, st.svc.Commands.Run.After)
		s.setStatus(st, StatusExited)
	}()
}

func (s *Supervisor) runCommandChain(st *serviceState, c config.Command) error {
	if err := s.runChain(st, c.Before); err != nil {
		return err
	}
	if c.Command != "" {
		if err := s.runTracked(st, c); err != nil {
			return err
		}
	}
	return s.runChain(st, c.After)
}

func (s *Supervisor) runChain(st *serviceState, cmds []config.Command) error {
	for _, c := range cmds {
		if c.Command == "" {
			continue
		}
		if err := s.runTracked(st, c); err != nil {
			return fmt.Errorf("failed to run command %q: %w", c.Command, err)
		}
	}
	return nil
}

// runTracked runs a transient (build, before, after) command while keeping the
// runner reachable via st.runner, so Stop's stopRunner sweep can kill it
// mid-build. Cleared on return.
func (s *Supervisor) runTracked(st *serviceState, c config.Command) error {
	if s.ctx.Err() != nil {
		return s.ctx.Err()
	}
	runner := exec.NewRunner(s.execConfig(st.svc, c))
	runner.SetTee(s.tee(st.svc.Name))
	st.mu.Lock()
	st.runner = runner
	st.mu.Unlock()
	// recheck: if Stop's sweep ran in the window above and saw a nil runner it
	// would have skipped this command, so stop it here.
	if s.ctx.Err() != nil {
		_ = runner.Stop()
		st.mu.Lock()
		if st.runner == runner {
			st.runner = nil
		}
		st.mu.Unlock()
		return s.ctx.Err()
	}
	err := runner.Run()
	st.mu.Lock()
	if st.runner == runner {
		st.runner = nil
	}
	st.mu.Unlock()
	return err
}

func (s *Supervisor) execConfig(svc config.Service, c config.Command) exec.Config {
	dir := filepath.Join(s.cfg.DirRoot, svc.Dir, c.Dir)
	return exec.Config{
		Name:    svc.Name,
		Dir:     dir,
		Command: c.Command,
		Env:     svc.Env,
		Stdin:   s.stdinEnabled,
	}
}

func (s *Supervisor) stopRunner(st *serviceState) {
	st.mu.Lock()
	r := st.runner
	st.runner = nil
	st.mu.Unlock()
	if r != nil {
		_ = r.Stop()
	}
}

func (s *Supervisor) stopService(st *serviceState) {
	s.setStatus(st, StatusStopped)
	s.stopRunner(st)
}

func (s *Supervisor) runCleanup(st *serviceState) {
	if st.svc.Commands.Run != nil && st.svc.Commands.Run.CommandCleanup != "" {
		c := config.Command{Command: st.svc.Commands.Run.CommandCleanup}
		log.Info("running cleanup", "service", st.svc.Name, "command", c.Command)
		_ = exec.NewRunner(s.execConfig(st.svc, c)).Run()
	}
}

func (s *Supervisor) cascadeRestart(st *serviceState) {
	for _, name := range st.dependents {
		if dep, ok := s.services[name]; ok {
			select {
			case dep.restart <- struct{}{}:
			default:
			}
		}
	}
}

// waitForDeps blocks until every dependency listed in reload_on_service has
// become ready (running or cleanly exited). If a dependency instead reaches a
// terminal failure state it returns an error so the dependent gives up rather
// than waiting forever. A context cancel (shutdown) unblocks with a nil error.
func (s *Supervisor) waitForDeps(st *serviceState) error {
	for _, dep := range st.svc.Reload.ReloadOnService {
		if _, ok := s.services[dep]; !ok {
			continue
		}
		for {
			status := s.Status(dep)
			if status == StatusRunning || status == StatusExited {
				break
			}
			if isTerminalFailure(status) {
				log.Error("dependency failed to start, dependent will not start", "service", st.svc.Name, "dependency", dep, "status", status)
				return fmt.Errorf("service %q cannot start: dependency %q crashed: %w", st.svc.Name, dep, ErrDependencyFailed)
			}
			select {
			case <-s.ctx.Done():
				return nil
			case <-time.After(50 * time.Millisecond):
			}
		}
	}
	return nil
}

func (s *Supervisor) forwardWatcher(name string, w *watcher.Watcher) {
	defer s.wg.Done()
	for ev := range w.Events() {
		log.Debug("watcher event", "service", name, "kind", ev.Kind, "paths", ev.Paths, "setup", ev.SetupTrigger)
		if ev.SetupTrigger {
			if st := s.services[name]; st != nil {
				st.setupPending.Store(true)
			}
		}
		_ = s.Restart(name)
	}
}

func (s *Supervisor) setStatus(st *serviceState, status Status) {
	st.mu.Lock()
	st.status = status
	st.mu.Unlock()
	s.publishStatus(st.svc.Name, "", status, nil)
}

func (s *Supervisor) setStatusErr(st *serviceState, status Status, err error) {
	st.mu.Lock()
	st.status = status
	st.mu.Unlock()
	s.publishStatus(st.svc.Name, "", status, err)
}

// publishStatus translates a supervisor-local status update into a
// protocol.StatusEvent and pushes it through the Hub. All status emission
// (service, managed child, restart marker) goes here.
func (s *Supervisor) publishStatus(service, child string, status Status, err error, ports ...int) {
	ev := protocol.StatusEvent{
		Service: service,
		Child:   child,
		Status:  string(status),
		At:      time.Now(),
		Ports:   ports,
	}
	if err != nil {
		ev.Err = err.Error()
	}
	s.hub.PublishStatus(ev)
}

// publishLog routes a captured line through the Hub. Used by the
// supervisor-owned tee on shell runners and by forwardManagerLogs for
// runtime-managed children.
func (s *Supervisor) publishLog(service, child, line string) {
	s.hub.PublishLog(protocol.LogLine{
		Service: service,
		Child:   child,
		Line:    line,
		At:      time.Now(),
	})
}

// tee returns an io.Writer that publishes each line written to it as a
// protocol.LogLine on the Hub. exec.Runner.captureOutput already writes one
// line per call, so this trims the trailing newline and forwards.
func (s *Supervisor) tee(service string) io.Writer {
	return supervisorTee{s: s, service: service}
}

type supervisorTee struct {
	s       *Supervisor
	service string
}

func (t supervisorTee) Write(p []byte) (int, error) {
	t.s.publishLog(t.service, "", strings.TrimRight(string(p), "\n"))
	return len(p), nil
}

func topoSort(services []config.Service) ([]string, error) {
	indegree := make(map[string]int, len(services))
	graph := make(map[string][]string, len(services))
	order := make([]string, 0, len(services))
	inputOrder := make(map[string]int, len(services))

	for i, svc := range services {
		indegree[svc.Name] = 0
		inputOrder[svc.Name] = i
	}
	for _, svc := range services {
		for _, dep := range svc.Reload.ReloadOnService {
			graph[dep] = append(graph[dep], svc.Name)
			indegree[svc.Name]++
		}
	}

	ready := make([]string, 0)
	for name, d := range indegree {
		if d == 0 {
			ready = append(ready, name)
		}
	}

	for len(ready) > 0 {
		sort.SliceStable(ready, func(i, j int) bool { return inputOrder[ready[i]] < inputOrder[ready[j]] })
		name := ready[0]
		ready = ready[1:]
		order = append(order, name)
		for _, next := range graph[name] {
			indegree[next]--
			if indegree[next] == 0 {
				ready = append(ready, next)
			}
		}
	}

	if len(order) != len(services) {
		return nil, errors.New("supervisor: dependency cycle detected in services")
	}
	return order, nil
}

// dispatchHooks runs every registered ServiceHook for the given phase against
// the service. Hook errors log at warn and never abort the lifecycle; a strict
// hook can surface failures via status events instead. A nil registry is a
// no-op.
func (s *Supervisor) dispatchHooks(phase addon.Phase, svc config.Service) {
	if !s.registry.HasHooks(phase) {
		return
	}
	s.registry.DispatchHooks(s.ctx, phase, s.cfg, svc, func(hook string, err error) {
		log.Warn("hook failed", "service", svc.Name, "phase", phase, "hook", hook, "error", err)
	})
}
