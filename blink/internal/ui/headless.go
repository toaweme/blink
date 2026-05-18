package ui

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/toaweme/blink/core/addon"
	"github.com/toaweme/blink/core/config"
	"github.com/toaweme/blink/core/output"
	"github.com/toaweme/blink/core/supervisor"
	"github.com/toaweme/log"
)

// Headless supervises services with no terminal rendering at all. It is
// the `-u headless` / `UI: none` mode: the supervisor and the control
// socket run as an always-on substrate, and (while log writing is enabled)
// captured output is tee'd to per-service files under Paths.LogDir so agents
// (or a later-attached viewer) can read it. With log writing off it runs as
// a pure supervisor. Slog stays active - nothing owns the screen.
type Headless struct {
	reg *addon.Registry

	mu     sync.Mutex
	sup    *supervisor.Supervisor
	cancel context.CancelFunc
}

var _ UserInterface = (*Headless)(nil)

func NewHeadless(reg *addon.Registry) *Headless {
	return &Headless{reg: reg}
}

func (h *Headless) Run(cfg config.Config) error {
	sup, err := supervisor.New(cfg, h.reg)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	h.mu.Lock()
	h.sup = sup
	h.cancel = cancel
	h.mu.Unlock()
	defer cancel()

	if err := os.MkdirAll(cfg.Paths.LogDir, 0o755); err != nil {
		return fmt.Errorf("failed to create log dir %q: %w", cfg.Paths.LogDir, err)
	}

	if err := sup.Start(ctx); err != nil {
		return err
	}
	log.Info("running headless", "services", len(cfg.Services), "logs", cfg.Paths.LogDir, "writing", cfg.LogWriteEnabled())

	sub, cancelSub := sup.Subscribe()
	defer cancelSub()

	sink := newLogSink(cfg.Paths.LogDir, cfg.LogWriteEnabled())
	go h.consumeStatus(sub)
	sink.consume(sub) // blocks until the hub closes or ctx cancels
	return nil
}

func (h *Headless) consumeStatus(sub output.Subscription) {
	for ev := range sub.Status {
		svc := ev.Service
		if ev.Child != "" {
			svc = ev.Service + "/" + ev.Child
		}
		if ev.Err != "" {
			log.Warn("service status", "service", svc, "status", ev.Status, "error", ev.Err)
			continue
		}
		log.Info("service status", "service", svc, "status", ev.Status)
	}
}

func (h *Headless) Stop(_ config.Config) error {
	h.mu.Lock()
	sup := h.sup
	cancel := h.cancel
	h.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if sup == nil {
		return nil
	}
	log.Info("stopping (headless)")
	return sup.Stop(context.Background())
}
