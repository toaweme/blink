package configform

import (
	"os"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
)

// KeyMap returns a huh KeyMap that lets either Esc or Ctrl+C abort any
// form, in addition to the default Ctrl+C alone. Without this, huh only
// binds Ctrl+C to Quit, and users hitting Esc (the universal back key)
// or q (the vim quit key) end up stuck inside a Select with no
// discoverable way out.
//
// Use it with every huh.NewForm call so navigation behaves the same
// across init, edit, and any future interactive surface.
//
// It also lets ↑/↓ move between fields (in addition to enter/tab), so a user
// doesn't have to submit a field with enter just to reach the next one. This is
// applied only to Input and Confirm fields: Select/MultiSelect keep ↑/↓ for
// moving through their options, and Text keeps them for moving within the
// multi-line value. Confirm already binds ←/→ to toggle its buttons.
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
// alternate screen so it renders over a clean buffer and leaves no residue
// behind on exit. Without alt-screen the form draws inline and its colourful
// output stays in the scrollback after the picker quits, so the user sees the
// leftover form instead of just the final notice. The picker (a bubbletea
// program) already uses alt-screen, so this keeps the editor consistent.
//
// WithProgramOptions replaces huh's default tea options, so the defaults it sets
// (stderr output, focus reporting) are re-applied alongside alt-screen.
func Run(form *huh.Form) error {
	return form.WithKeyMap(KeyMap()).WithShowHelp(true).
		WithProgramOptions(
			tea.WithOutput(os.Stderr),
			tea.WithReportFocus(),
			tea.WithAltScreen(),
		).Run()
}
