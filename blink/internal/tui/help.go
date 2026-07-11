package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/toaweme/blink/blink/internal/theme"
)

// humanizeKey renders a bubbletea key string for display: " " becomes "space"
// and the shift+arrow combos use glyphs.
func humanizeKey(k string) string {
	switch k {
	case " ":
		return "space"
	case "shift+down":
		return "shift+↓"
	case "shift+up":
		return "shift+↑"
	default:
		return k
	}
}

// renderHelpDialog renders the keyboard reference modal. Bindings are rendered
// live from the active keymap so blink.yaml control.keys overrides are reflected.
func (m *Model) renderHelpDialog() string {
	titleStyle := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
	dim := lipgloss.NewStyle().Foreground(theme.Muted)

	header := titleStyle.Render("BLINK") + "  " + dim.Render("keyboard · esc close")
	body := m.renderHelpKeyboard()

	// modalScroll-aware viewport so a long body doesn't get clipped.
	lines := strings.Split(body, "\n")
	innerH := m.height - 10
	if innerH < 4 {
		innerH = 4
	}
	maxScroll := len(lines) - innerH
	if maxScroll < 0 {
		maxScroll = 0
	}
	scroll := m.modalScroll
	if scroll > maxScroll {
		scroll = maxScroll
	}
	if scroll < 0 {
		scroll = 0
	}
	end := scroll + innerH
	if end > len(lines) {
		end = len(lines)
	}
	visible := lines[scroll:end]
	scrollHint := ""
	if maxScroll > 0 {
		scrollHint = dim.Render(fmt.Sprintf("  · %d/%d", scroll+1, maxScroll+1))
	}

	content := header + scrollHint + "\n\n" + strings.Join(visible, "\n")
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Accent).
		Padding(1, 3).
		Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m *Model) renderHelpKeyboard() string {
	// section headings are set apart by weight, not a distinct hue, so the modal
	// stays on-palette (accent title, yellow keys, neutral text).
	title := lipgloss.NewStyle().Foreground(theme.Bright).Bold(true)
	keyStyle := lipgloss.NewStyle().Foreground(theme.Cursor).Bold(true)
	desc := lipgloss.NewStyle().Foreground(theme.Subtle)

	pair := func(k, d string) string { return "  " + padRight(keyStyle.Render(k), 22) + desc.Render(d) }

	// bindings are rendered live from the active keymap so control.keys overrides
	// are reflected.
	lines := []string{title.Render("bindings")}
	for _, e := range m.keymap.Help() {
		keys := make([]string, len(e.Keys))
		for i, k := range e.Keys {
			keys[i] = humanizeKey(k)
		}
		lines = append(lines, pair(strings.Join(keys, " / "), e.Help))
	}

	// fixed keys not part of the rebindable keymap.
	lines = append(lines,
		"",
		title.Render("navigation (fixed)"),
		pair("1-9", "jump to tab"),
		pair("↑ / ↓", "scroll"),
		pair("pgup / pgdn", "page up / down"),
		pair("home / end", "scroll to top / bottom"),
	)
	return strings.Join(lines, "\n")
}
