package configform

import (
	"github.com/charmbracelet/bubbles/key"
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
func KeyMap() *huh.KeyMap {
	km := huh.NewDefaultKeyMap()
	km.Quit = key.NewBinding(
		key.WithKeys("esc", "ctrl+c"),
		key.WithHelp("esc", "back"),
	)
	return km
}

// Run executes a huh form with the shared key map and help enabled.
// Centralized so the keymap stays in sync across every prompt.
func Run(form *huh.Form) error {
	return form.WithKeyMap(KeyMap()).WithShowHelp(true).Run()
}
