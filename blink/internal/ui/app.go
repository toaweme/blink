// Package ui provides blink's rendering backends (the full-screen TUI, the
// plain stdout printer, and the headless supervisor) and the App that selects
// among them based on config.
package ui

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/toaweme/log"

	"github.com/toaweme/blink/core/addon"
	"github.com/toaweme/blink/core/config"
)

// UserInterface is one of the rendering backends blink supports.
type UserInterface interface {
	Run(cfg config.Config) error
	Stop(cfg config.Config) error
}

// App selects a UserInterface based on the config and handles signal shutdown.
type App struct {
	UIs map[string]UserInterface
}

// NewApp returns an App that dispatches to one of the given UI backends.
func NewApp(uis map[string]UserInterface) *App { return &App{UIs: uis} }

// DefaultRegistry returns the UI implementations shipped with blink. The
// addon registry is forwarded to the supervisor each UI builds, so runtimes
// and global lifecycle hooks (portkill, etc.) apply consistently across UIs.
func DefaultRegistry(reg *addon.Registry) map[string]UserInterface {
	return map[string]UserInterface{
		"blink":    NewBlink(reg),
		"plain":    NewPlain(reg),
		"headless": NewHeadless(reg),
	}
}

// Run dispatches to the configured UI. When config.UI is unset, it picks "plain"
// when stdout is not a TTY (e.g. piped output / CI) and "blink" otherwise.
func (a *App) Run(cfg config.Config) error {
	if cfg.UI == "" {
		if PlainIsAppropriate() {
			cfg.UI = "plain"
		} else {
			cfg.UI = "blink"
		}
	}
	// "none" is the documented yaml spelling of the headless mode.
	if cfg.UI == "none" {
		cfg.UI = "headless"
	}

	ui, ok := a.UIs[cfg.UI]
	if !ok {
		return fmt.Errorf("ui %q not found", cfg.UI)
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	done := make(chan error, 1)
	go func() { done <- ui.Run(cfg) }()

	select {
	case sig := <-sigs:
		log.Info("received signal", "signal", sig.String())
		if err := ui.Stop(cfg); err != nil {
			log.Error("ui stop failed", "error", err)
		}
		return <-done
	case err := <-done:
		return err
	}
}
