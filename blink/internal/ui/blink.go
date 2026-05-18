package ui

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/toaweme/blink/blink/internal/tui"
	"github.com/toaweme/blink/core/addon"
	"github.com/toaweme/blink/core/config"
	"github.com/toaweme/blink/core/control"
	"github.com/toaweme/blink/core/output"
	"github.com/toaweme/blink/core/supervisor"
	"github.com/toaweme/log"
)

// Blink is the full-screen tabbed TUI built on bubbletea.
type Blink struct {
	mu  sync.Mutex
	reg *addon.Registry
	sup *supervisor.Supervisor
	app *tui.App
}

var _ UserInterface = (*Blink)(nil)

func NewBlink(reg *addon.Registry) *Blink {
	return &Blink{reg: reg}
}

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
	// model gets the dir + initial state + a toggle so `L` flips it live.
	sink := newLogSink(cfg.Paths.LogDir, cfg.LogWriteEnabled())
	model := tui.NewModel(sup.Order(), controllerAdapter{sup: sup}).
		WithKeymap(km).
		WithZen(cfg.Zen).
		WithLogControl(cfg.Paths.LogDir, sink.Enabled(), sink.Toggle)
	app := tui.NewApp(model)

	b.mu.Lock()
	b.sup = sup
	b.app = app
	b.mu.Unlock()

	// Suppress slog output to stdout while the TUI owns the screen. Routing
	// log messages into the TUI happens via tee writers below; everything
	// emitted by toaweme/log goes to /dev/null for the duration of the run.
	silenceSlog()
	defer restoreSlog()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := sup.Start(ctx); err != nil {
		return err
	}

	sub, cancelSub := sup.Subscribe()
	defer cancelSub()
	logSub, cancelLogSub := sup.Subscribe()
	defer cancelLogSub()
	go forwardEvents(app, sub)
	go forwardLogs(app, sub)
	go sink.consume(logSub)
	go pollWatchStats(ctx, app, sup)

	err = app.Run()

	// Tear down regardless of how the TUI exited (user quit, panic-recovery, etc).
	_ = sup.Stop(ctx)
	return err
}

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

// forwardEvents posts status events from the supervisor's hub into the
// bubbletea program. Child events (e.g. docker container state changes)
// pass through unmodified so the model renders them nested under their
// parent service.
func forwardEvents(app *tui.App, sub output.Subscription) {
	for ev := range sub.Status {
		var err error
		if ev.Err != "" {
			err = errString(ev.Err)
		}
		app.Send(tui.StatusMsg{
			Service: ev.Service,
			Child:   ev.Child,
			Status:  ev.Status,
			Err:     err,
		})
	}
}

// forwardLogs posts log lines from the supervisor's hub into the TUI.
// Shell-runtime output, docker compose container output, and any other
// captured stream all flow through the same channel.
func forwardLogs(app *tui.App, sub output.Subscription) {
	for ln := range sub.Logs {
		app.Send(tui.LineMsg{Service: ln.Service, Child: ln.Child, Line: ln.Line})
	}
}

// pollWatchStats pushes the supervisor's aggregate watch counts into
// the TUI on a slow cadence. The first message lands quickly (~1s after
// start, by which point initial filesystem walks have completed) and
// then every 5s to catch directories added at runtime.
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

// errString turns a protocol-wire error string back into an error so the
// TUI's StatusMsg consumer can keep its existing error-typed field.
type errString string

func (e errString) Error() string { return string(e) }

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
	}
	return nil
}
