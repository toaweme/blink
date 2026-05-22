package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/toaweme/blink/core/control"
)

// App wraps the bubbletea program so callers can send external messages
// (log lines, status updates) from goroutines.
type App struct {
	prog  *tea.Program
	model *Model
}

// WithZen starts the TUI in zen / raw mode when on is true. Same end-state
// as pressing z at runtime, so the z exit path works unchanged.
func (m *Model) WithZen(on bool) *Model {
	m.rawMode = on
	return m
}

// WithKeymap replaces the model's keymap, e.g. one merged from blink.yaml's
// control.keys.
func (m *Model) WithKeymap(km control.Keymap) *Model {
	m.keymap = km
	return m
}

// WithLogControl wires the log sink into the model: logDir is where selection
// writes land (<svc>.selected.log), on is the footer indicator's initial state,
// and toggle flips the sink live (the L key), returning the new state. A nil
// toggle makes the L key a no-op (read-only modes).
func (m *Model) WithLogControl(logDir string, on bool, toggle func() bool) *Model {
	m.logDir = logDir
	m.logsOn = on
	m.logToggle = toggle
	return m
}

// NewApp wraps a model in a runnable bubbletea program with the alt-screen
// and mouse capture enabled.
func NewApp(model *Model) *App {
	prog := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	return &App{prog: prog, model: model}
}

// Program returns the underlying bubbletea program.
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
