package ui

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/toaweme/log"

	"github.com/toaweme/blink/blink/internal/tui"
	"github.com/toaweme/blink/core/addon"
	"github.com/toaweme/blink/core/config"
	"github.com/toaweme/blink/core/control"
	"github.com/toaweme/blink/core/output"
	"github.com/toaweme/blink/core/supervisor"
)

// Blink is the full-screen tabbed TUI built on bubbletea.
type Blink struct {
	mu  sync.Mutex
	reg *addon.Registry
	sup *supervisor.Supervisor
	app *tui.App
}

var _ UserInterface = (*Blink)(nil)

// NewBlink returns a Blink UI backed by the given addon registry.
func NewBlink(reg *addon.Registry) *Blink {
	return &Blink{reg: reg}
}

// Run starts the supervisor and the bubbletea TUI, blocking until the user quits.
func (b *Blink) Run(cfg config.Config) error {
	sup, err := supervisor.New(cfg, b.reg)
	if err != nil {
		return err
	}

	km, err := control.DefaultKeymap().Merge(cfg.Control.Keys)
	if err != nil {
		return fmt.Errorf("failed to apply control.keys: %w", err)
	}
	// log writing is a Hub subscriber, orthogonal to the TUI render path. The
	// model gets the dir, initial state, and a toggle so L flips it live.
	sink := newLogSink(cfg.Paths.LogDir, cfg.LogWriteEnabled())
	model := tui.NewModel(sup.Order(), controllerAdapter{sup: sup}).
		WithKeymap(km).
		WithZen(cfg.Zen).
		WithServiceURLs(serviceURLs(cfg)).
		WithLogControl(cfg.Paths.LogDir, sink.Enabled(), sink.Toggle)
	app := tui.NewApp(model)

	b.mu.Lock()
	b.sup = sup
	b.app = app
	b.mu.Unlock()

	// suppress slog output while the TUI owns the screen; everything emitted by
	// toaweme/log is discarded for the duration of the run.
	silenceSlog()
	defer restoreSlog()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// subscribe (and wire the forwarders) before Start: a fast shell/go service
	// can reach "running" or "crashed" before the first subscriber exists, and
	// the Hub drops any event with no subscriber. Subscribing first latches every
	// boot-time status and log line into the buffered channels; the forwarders
	// drain them once app.Run begins.
	sub, cancelSub := sup.Subscribe()
	defer cancelSub()
	logSub, cancelLogSub := sup.Subscribe()
	defer cancelLogSub()
	go forwardEvents(app, sub)
	go forwardLogs(app, sub)
	go sink.consume(logSub)
	go pollWatchStats(ctx, app, sup)

	if err := sup.Start(ctx); err != nil {
		return err
	}

	err = app.Run()

	// tear down regardless of how the TUI exited.
	_ = sup.Stop(ctx)
	return err
}

// Stop quits the running program and tears down the supervisor.
func (b *Blink) Stop(_ config.Config) error {
	b.mu.Lock()
	app := b.app
	sup := b.sup
	b.mu.Unlock()
	if app != nil {
		app.Quit()
	}
	if sup != nil {
		log.Info("stopping (blink UI)")
		return sup.Stop(context.Background())
	}
	return nil
}

// forwardEvents posts status events from the supervisor's hub into the bubbletea
// program. Child events (e.g. docker container state changes) pass through
// unmodified so the model renders them nested under their parent service.
func forwardEvents(app *tui.App, sub output.Subscription) {
	for ev := range sub.Status {
		var err error
		if ev.Err != "" {
			err = stringError(ev.Err)
		}
		app.Send(tui.StatusMsg{
			Service: ev.Service,
			Child:   ev.Child,
			Status:  ev.Status,
			Err:     err,
		})
	}
}

// forwardLogs posts log lines from the supervisor's hub into the TUI. All
// captured streams (shell-runtime, docker compose, etc.) share one channel.
func forwardLogs(app *tui.App, sub output.Subscription) {
	for ln := range sub.Logs {
		app.Send(tui.LineMsg{Service: ln.Service, Child: ln.Child, Line: ln.Line})
	}
}

// pollWatchStats pushes the supervisor's watch counts into the TUI on a slow
// cadence: the first message ~1s after start (once initial filesystem walks
// complete), then every 5s to catch directories added at runtime.
func pollWatchStats(ctx context.Context, app *tui.App, sup *supervisor.Supervisor) {
	send := func() {
		files, dirs := sup.WatchStats()
		per := sup.WatchStatsByService()
		out := make(map[string]tui.WatchStat, len(per))
		for k, v := range per {
			out[k] = tui.WatchStat{Files: v.Files, Dirs: v.Dirs}
		}
		app.Send(tui.WatchStatsMsg{Files: files, Dirs: dirs, PerSvc: out})
	}
	select {
	case <-ctx.Done():
		return
	case <-time.After(time.Second):
		send()
	}
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			send()
		}
	}
}

// serviceURLs maps each service that binds a known port to a local URL, keyed
// by service name. Ports come from the service's probed/configured Ports,
// resolved against its env (env-referenced ports fall back to the process
// environment, where .env was loaded). The first resolvable port wins; a
// service with none is omitted, so the footer shows nothing for it.
func serviceURLs(cfg config.Config) map[string]string {
	urls := make(map[string]string, len(cfg.Services))
	for _, svc := range cfg.Services {
		ports := config.ResolvePorts(svc.Ports, svc.Env)
		if len(ports) == 0 {
			continue
		}
		urls[svc.Name] = fmt.Sprintf("http://127.0.0.1:%d", ports[0])
	}
	return urls
}

// stringError turns a protocol-wire error string back into an error so the
// TUI's StatusMsg consumer can keep its existing error-typed field.
type stringError string

func (e stringError) Error() string { return string(e) }

// controllerAdapter dispatches the TUI's session actions straight into the
// local supervisor.
type controllerAdapter struct{ sup *supervisor.Supervisor }

func (c controllerAdapter) Dispatch(action control.Action, service string) error {
	switch action {
	case control.ActionRestart:
		return c.sup.Restart(service)
	case control.ActionRestartAll:
		c.sup.RestartAll()
	case control.ActionInsertBlank:
		c.sup.InsertBlank(service)
	default:
		// view-only actions never reach the controller; ignore anything else.
	}
	return nil
}
