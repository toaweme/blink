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

// WithZen starts the TUI in zen (chromeless) mode when on is true. Same
// end-state as pressing z at runtime, so the z toggle works unchanged.
func (m *Model) WithZen(on bool) *Model {
	m.chromeless = on
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

// WithServicePorts seeds the local TCP ports each service listens on, keyed by
// service name (e.g. "web" -> {8080}). The footer renders them as an address on
// that service's tab, left of the uptime. Runtime-managed services (docker)
// whose ports are only known once the stack is up are filled in later from
// their status events; services with no known port show no address.
func (m *Model) WithServicePorts(ports map[string][]int) *Model {
	m.ports = ports
	return m
}

// WithProjectPath records the project root shown (shortened to its last two
// path segments) on the right of the help modal header, so several concurrent
// blink instances can be told apart at a glance. Empty leaves it hidden.
func (m *Model) WithProjectPath(path string) *Model {
	m.projectPath = path
	return m
}

// NewApp wraps a model in a runnable bubbletea program in the alt-screen.
// Mouse capture stays off so the terminal keeps native text selection. With it
// on, every drag goes to the app instead and select-to-copy breaks. The wheel
// still scrolls because the terminal's alternate-scroll feeds it in as up/down.
func NewApp(model *Model) *App {
	prog := tea.NewProgram(model, tea.WithAltScreen())
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
