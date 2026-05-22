package configform

import (
	"os"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
)

// KeyMap returns a huh KeyMap that lets either Esc or Ctrl+C abort any form, in
// addition to the default Ctrl+C alone. By default huh binds only Ctrl+C to
// Quit, leaving a user who hits Esc stuck inside a Select. Use it with every
// huh.NewForm call so navigation is consistent across init, edit, and future
// interactive surfaces.
//
// It also lets ↑/↓ move between fields (in addition to enter/tab), applied only
// to Input and Confirm: Select/MultiSelect keep ↑/↓ for their options, and Text
// keeps them for moving within the multi-line value.
func KeyMap() *huh.KeyMap {
	km := huh.NewDefaultKeyMap()
	km.Quit = key.NewBinding(
		key.WithKeys("esc", "ctrl+c"),
		key.WithHelp("esc", "back"),
	)

	km.Input.Next = key.NewBinding(key.WithKeys("enter", "tab", "down"), key.WithHelp("↓", "next"))
	km.Input.Prev = key.NewBinding(key.WithKeys("shift+tab", "up"), key.WithHelp("↑", "back"))
	km.Confirm.Next = key.NewBinding(key.WithKeys("enter", "tab", "down"), key.WithHelp("↓", "next"))
	km.Confirm.Prev = key.NewBinding(key.WithKeys("shift+tab", "up"), key.WithHelp("↑", "back"))

	return km
}

// Run executes a huh form with the shared key map and help enabled, in the
// alternate screen so it renders over a clean buffer and leaves no residue on
// exit. Without alt-screen the form draws inline and lingers in the scrollback
// after the picker quits. The picker (a bubbletea program) already uses
// alt-screen, so this keeps the editor consistent.
//
// WithProgramOptions replaces huh's default tea options, so its defaults
// (stderr output, focus reporting) are re-applied alongside alt-screen.
func Run(form *huh.Form) error {
	return form.WithKeyMap(KeyMap()).WithShowHelp(true).
		WithProgramOptions(
			tea.WithOutput(os.Stderr),
			tea.WithReportFocus(),
			tea.WithAltScreen(),
		).Run()
}
