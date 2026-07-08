// Package theme is the single source of truth for blink's terminal colors.
//
// Every UI package (tui, configform, ui/plain) pulls its colors from here so the
// brand accent, status semantics, and chrome tints stay consistent instead of
// drifting into near-identical shades scattered across the tree.
//
// Colors are lipgloss.AdaptiveColor: each role carries a Dark variant (used on a
// dark terminal) and a Light variant (used on a light one), so the palette reads
// on both. lipgloss picks the variant from the detected terminal background. The
// scheme is anchored on blink's original teal/cyan logo, with dark-emerald greens
// and muted, non-neon hues throughout.
package theme

import "github.com/charmbracelet/lipgloss"

var (
	// Accent is the one brand/active/selected teal, taken from the logo and shared
	// by the active tab, the logo's resolving hue, the "blink init" title, the form
	// cursor, the help border, and the focused-container badge.
	Accent = lipgloss.AdaptiveColor{Light: "#0f766e", Dark: "#14b8a6"}
	// OnAccent is the contrasting text on a filled Accent or status chip: near-black
	// on a dark terminal, white on a light one.
	OnAccent = lipgloss.AdaptiveColor{Light: "#ffffff", Dark: "#04120f"}

	// Success is the running/selected/done status color, a dark emerald.
	Success = lipgloss.AdaptiveColor{Light: "#047857", Dark: "#059669"}
	// Warning is the transient-mode status color (amber).
	Warning = lipgloss.AdaptiveColor{Light: "#b45309", Dark: "#d99a2b"}
	// Danger is the crashed/failed status color (red).
	Danger = lipgloss.AdaptiveColor{Light: "#c0362c", Dark: "#e0575b"}

	// Muted is the dim-label / pending-state neutral.
	Muted = lipgloss.AdaptiveColor{Light: "#6b7280", Dark: "#8b909a"}
	// Faint is the faintest chrome neutral (column headers).
	Faint = lipgloss.AdaptiveColor{Light: "#9aa0a8", Dark: "#5f646e"}
	// Subtle is the readable secondary-text neutral.
	Subtle = lipgloss.AdaptiveColor{Light: "#3f4650", Dark: "#c6cad3"}
	// Bright is the emphasized-key / value neutral.
	Bright = lipgloss.AdaptiveColor{Light: "#1f2430", Dark: "#eceef2"}

	// Rule is the thin horizontal separator.
	Rule = lipgloss.AdaptiveColor{Light: "#d3d7dd", Dark: "#3a3f47"}
	// BarBg tints the footer bar.
	BarBg = lipgloss.AdaptiveColor{Light: "#e7e9ed", Dark: "#23272e"}

	// Cursor is the gold gutter marker, spinner, and key-hint highlight.
	Cursor = lipgloss.AdaptiveColor{Light: "#a16207", Dark: "#e0b341"}

	// Link is the loopback URL and literal-port blue.
	Link = lipgloss.AdaptiveColor{Light: "#1d6fb8", Dark: "#4aa3e0"}
	// Env tints env-var port references and picker notices.
	Env = lipgloss.AdaptiveColor{Light: "#6d5bb0", Dark: "#b39ddb"}
)

// LogoRamp is the per-letter gradient for the "blink" wordmark: a teal-only ramp
// that deepens from a dark teal and resolves on Accent, so it settles onto the
// brand color without ever dipping into blue.
var LogoRamp = []lipgloss.AdaptiveColor{
	{Light: "#0b5f58", Dark: "#0d9488"},
	{Light: "#0d6b62", Dark: "#12a594"},
	Accent, Accent, Accent,
}

// ServicePalette is the categorical set of foreground colors deterministically
// assigned to service names, stable across runs. The hues are distinct but sit in
// one muted jewel-tone family so interleaved output reads by service without
// shouting; this is unrelated to the status colors above.
var ServicePalette = []lipgloss.AdaptiveColor{
	{Light: "#1d6fb8", Dark: "#4a90d9"}, // blue
	{Light: "#9c3f9a", Dark: "#c678c4"}, // magenta
	{Light: "#6d4bb0", Dark: "#9a7bd4"}, // purple
	{Light: "#047857", Dark: "#10b981"}, // emerald
	{Light: "#bd3d45", Dark: "#e06c75"}, // rose
	{Light: "#a16207", Dark: "#d9a521"}, // amber
	{Light: "#0f766e", Dark: "#2bb0a5"}, // teal
	{Light: "#b84a7e", Dark: "#e07aa8"}, // pink
}

// ServiceTintPalette is the subdued background counterpart of ServicePalette: one
// tint per service slot, used in the all-tab to group lines by service without
// obscuring the text (dark tints on a dark terminal, pale washes on a light one).
// Index-aligned with ServicePalette.
var ServiceTintPalette = []lipgloss.AdaptiveColor{
	{Light: "#dbe8f5", Dark: "#16283d"}, // blue
	{Light: "#f2dcf0", Dark: "#331b32"}, // magenta
	{Light: "#e6dff5", Dark: "#241b3d"}, // purple
	{Light: "#d6f0e4", Dark: "#0d2b20"}, // emerald
	{Light: "#f7dcde", Dark: "#3a1b1e"}, // rose
	{Light: "#f5ebd0", Dark: "#332912"}, // amber
	{Light: "#d6f0ee", Dark: "#0d2b2a"}, // teal
	{Light: "#f7dce8", Dark: "#351f2a"}, // pink
}
