package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/toaweme/blink/core/control"
)

// App wraps the bubbletea program so callers can keep a reference to it for
// sending external messages (log lines, status updates) from goroutines.
type App struct {
	prog  *tea.Program
	model Model
}

// WithZen starts the TUI in zen / raw mode when on=true. The host CLI
// sets this from `blink -z` or `cfg.Zen`. Identical end-state to the
// user pressing `z` at runtime, so the existing exit path (`z` again)
// works the same way.
func (m Model) WithZen(on bool) Model {
	m.rawMode = on
	return m
}

// WithKeymap replaces the model's keymap (e.g. with one merged from
// blink.yaml's control.keys). Pass control.DefaultKeymap() merged via
// Merge; the host CLI is responsible for surfacing a Merge error.
func (m Model) WithKeymap(km control.Keymap) Model {
	m.keymap = km
	return m
}

// WithLogControl wires the host-side log sink into the model: logDir is
// where selection writes land (<svc>.selected.log), on is the sink's initial
// state for the footer indicator, and toggle flips the sink live (the `L`
// key) returning the new state. Pass a nil toggle for read-only modes (a
// remote mirror) - the key then does nothing.
func (m Model) WithLogControl(logDir string, on bool, toggle func() bool) Model {
	m.logDir = logDir
	m.logsOn = on
	m.logToggle = toggle
	return m
}

func NewApp(model Model) *App {
	prog := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	return &App{prog: prog, model: model}
}

func (a *App) Program() *tea.Program { return a.prog }

// Run blocks until the user quits.
func (a *App) Run() error {
	if _, err := a.prog.Run(); err != nil {
		return fmt.Errorf("failed to run tui: %w", err)
	}
	return nil
}

// Send forwards a message from a non-UI goroutine into the program.
func (a *App) Send(msg tea.Msg) { a.prog.Send(msg) }

// Quit asks the program to exit.
func (a *App) Quit() { a.prog.Quit() }
