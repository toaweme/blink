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
	Run(config config.Config) error
	Stop(config config.Config) error
}

// App selects a UserInterface based on the config and handles signal shutdown.
type App struct {
	UIs map[string]UserInterface
}

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
func (a *App) Run(config config.Config) error {
	if config.UI == "" {
		if PlainIsAppropriate() {
			config.UI = "plain"
		} else {
			config.UI = "blink"
		}
	}
	// "none" is the documented yaml spelling of the headless mode.
	if config.UI == "none" {
		config.UI = "headless"
	}

	ui, ok := a.UIs[config.UI]
	if !ok {
		return fmt.Errorf("ui %q not found", config.UI)
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	done := make(chan error, 1)
	go func() { done <- ui.Run(config) }()

	select {
	case sig := <-sigs:
		log.Info("received signal", "signal", sig.String())
		if err := ui.Stop(config); err != nil {
			log.Error("ui stop failed", "error", err)
		}
		return <-done
	case err := <-done:
		return err
	}
}
