package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/toaweme/blink/blink/internal/theme"
)

// shortProjectPath keeps the last two segments of a path (e.g. "toaweme/blink"),
// which is enough to tell several concurrent blink instances apart without
// widening the modal. A single-segment path is returned as-is.
func shortProjectPath(p string) string {
	if p == "" {
		return ""
	}
	p = filepath.Clean(p)
	base := filepath.Base(p)
	parent := filepath.Base(filepath.Dir(p))
	if parent == "." || parent == string(filepath.Separator) || parent == base {
		return base
	}
	return parent + "/" + base
}

// truncLeft clips s to limit display columns from the left, keeping the tail
// (the most specific segment) and marking the cut with a leading ellipsis.
func truncLeft(s string, limit int) string {
	r := []rune(s)
	if limit <= 0 {
		return ""
	}
	if len(r) <= limit {
		return s
	}
	if limit == 1 {
		return "…"
	}
	return "…" + string(r[len(r)-(limit-1):])
}

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

	left := titleStyle.Render("BLINK") + "  " + dim.Render("keyboard · esc close")
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
	if maxScroll > 0 {
		left += dim.Render(fmt.Sprintf("  · %d/%d", scroll+1, maxScroll+1))
	}

	header := m.renderHelpHeader(left, visible)
	content := header + "\n\n" + strings.Join(visible, "\n")
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Accent).
		Padding(1, 3).
		Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

// renderHelpHeader lays the modal title on the left and the shortened project
// path on the right of one line, so several concurrent blink instances are easy
// to tell apart. The path is right-aligned to the modal's content width (never
// widening the box for a short path) and clipped from the left if the terminal
// is too narrow to fit both.
func (m *Model) renderHelpHeader(left string, visible []string) string {
	if m.projectPath == "" {
		return left
	}
	dim := lipgloss.NewStyle().Foreground(theme.Muted)
	short := shortProjectPath(m.projectPath)

	contentWidth := 0
	for _, ln := range visible {
		if w := lipgloss.Width(ln); w > contentWidth {
			contentWidth = w
		}
	}
	leftWidth := lipgloss.Width(left)

	// keep at least one space between the title and the path.
	const minGap = 2
	target := contentWidth
	if want := leftWidth + minGap + len([]rune(short)); want > target {
		target = want
	}
	// the box adds a rounded border (1 col each side) and Padding(1, 3) (3 cols
	// each side), so 8 cols of chrome sit around the content.
	if avail := m.width - 8; avail > 0 && target > avail {
		target = avail
	}

	pathWidth := target - leftWidth - minGap
	if pathWidth < 1 {
		// no room for the path on this terminal, show the title alone.
		return left
	}
	short = truncLeft(short, pathWidth)
	gap := target - leftWidth - lipgloss.Width(dim.Render(short))
	if gap < minGap {
		gap = minGap
	}
	return left + strings.Repeat(" ", gap) + dim.Render(short)
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

	// fixed keys not part of the rebindable keymap. up/down are rebindable
	// (cursor-up/cursor-down) and render in the bindings section above, so they
	// are deliberately absent here.
	lines = append(lines,
		"",
		title.Render("navigation (fixed)"),
		pair("1-9", "jump to tab"),
		pair("pgup / pgdn", "page up / down"),
		pair("home / end", "scroll to top / bottom"),
		pair("mouse / touchpad", "scroll"),
	)
	return strings.Join(lines, "\n")
}
