package control

// Scope separates the two kinds of action the TUI can bind a key to.
type Scope int

const (
	// ScopeSession actions act on the supervised services and are
	// role-gated and wire-encodable: they dispatch identically whether the
	// consumer is the local TUI (in-process Controller) or a remote mirror
	// (session.Client over a transport). They reuse the wire Verb strings.
	ScopeSession Scope = iota
	// ScopeView actions are consumer-local presentation concerns (scroll,
	// switch tab, toggle zen, quit). They never cross the wire.
	ScopeView
)

// Action is the stable identifier a key binds to. Session actions reuse
// the wire Verb* strings so a binding and a command share one name.
type Action string

const (
	// session actions (role-gated, wire-encodable). These reuse the wire
	// Verb* strings so a key binding and the command it dispatches share
	// one identity. Only the actions actually bound to keys live here;
	// other verbs (list, signal, send, dump-logs, resync) are issued over
	// the wire directly and have no keybinding.
	ActionRestart           = Action(VerbRestart)
	ActionRestartAll Action = "restart-all"
	// ActionInsertBlank publishes a blank line into the focused service's
	// output stream (buffer + log file). Session-scoped: it mutates the
	// shared Hub output, not just the local view.
	ActionInsertBlank Action = "insert-blank"

	// view actions (consumer-local, never wire).
	ActionQuit          Action = "quit"
	ActionCommandCenter Action = "command-center"
	ActionToggleZen     Action = "toggle-zen"
	ActionToggleLogs    Action = "toggle-logs"
	ActionNextTab       Action = "next-tab"
	ActionPrevTab       Action = "prev-tab"
	// container-focus actions cycle which child of a runtime-managed service
	// (docker compose containers) the active tab shows. No-op on tabs without
	// children.
	ActionNextChild Action = "next-child"
	ActionPrevChild Action = "prev-child"
	// tab-history actions walk the visited-tab trail (browser back/forward):
	// after jumping from tab 1 to tab 4, back returns to tab 1. Distinct from
	// next/prev-tab, which step to the adjacent tab.
	ActionHistBack    Action = "hist-back"
	ActionHistForward Action = "hist-forward"
	ActionClear         Action = "clear"
	ActionClearAll      Action = "clear-all"
	// cursor-mode actions. The line cursor is a mode (ActionCursorMode
	// toggles it). While off, cursor-up/down scroll the viewport; while on
	// they move the cursor and the selection keys are live.
	ActionCursorMode      Action = "cursor-mode"
	ActionCursorUp        Action = "cursor-up"
	ActionCursorDown      Action = "cursor-down"
	ActionExtendUp        Action = "extend-up"
	ActionExtendDown      Action = "extend-down"
	ActionToggleSelect    Action = "toggle-select"
	ActionCopy            Action = "copy"
	ActionClearCursor     Action = "clear-cursor"
	ActionWriteSelection  Action = "write-selection"
	ActionAppendSelection Action = "append-selection"
)

// Spec describes one action: its scope, the minimum role required to
// dispatch it (session actions only), and a one-line help string. The
// catalog is the single source of truth a Keymap validates against.
type Spec struct {
	Action Action
	Scope  Scope
	Role   Role
	Help   string
}

// Actions returns the closed catalog of bindable actions. A blink.yaml
// control.keys override that names an action absent here is rejected by
// Keymap.Merge.
func Actions() []Spec {
	return []Spec{
		{ActionRestart, ScopeSession, RoleOperator, "restart the focused service"},
		{ActionRestartAll, ScopeSession, RoleOperator, "restart all services"},
		{ActionInsertBlank, ScopeSession, RoleOperator, "insert a blank line into the focused service's output"},
		{ActionNextTab, ScopeView, RoleViewer, "next tab"},
		{ActionPrevTab, ScopeView, RoleViewer, "previous tab"},
		{ActionNextChild, ScopeView, RoleViewer, "focus the next container (docker tab)"},
		{ActionPrevChild, ScopeView, RoleViewer, "focus the previous container (docker tab)"},
		{ActionHistBack, ScopeView, RoleViewer, "back to the previously viewed tab"},
		{ActionHistForward, ScopeView, RoleViewer, "forward in tab history"},
		{ActionClear, ScopeView, RoleViewer, "clear the focused tab buffer"},
		{ActionClearAll, ScopeView, RoleViewer, "clear all buffers"},
		{ActionCursorMode, ScopeView, RoleViewer, "toggle line-export mode"},
		{ActionCursorUp, ScopeView, RoleViewer, "scroll up (cursor up in cursor mode)"},
		{ActionCursorDown, ScopeView, RoleViewer, "scroll down (cursor down in cursor mode)"},
		{ActionExtendUp, ScopeView, RoleViewer, "extend selection up"},
		{ActionExtendDown, ScopeView, RoleViewer, "extend selection down"},
		{ActionToggleSelect, ScopeView, RoleViewer, "toggle the cursor line in the selection"},
		{ActionCopy, ScopeView, RoleViewer, "copy selection (or cursor line) to the clipboard"},
		{ActionClearCursor, ScopeView, RoleViewer, "clear selection / exit cursor mode"},
		{ActionWriteSelection, ScopeView, RoleViewer, "rewrite <svc>.selected.log with the selection"},
		{ActionAppendSelection, ScopeView, RoleViewer, "append the selection to <svc>.selected.log"},
		{ActionToggleLogs, ScopeView, RoleViewer, "toggle log-file writing"},
		{ActionCommandCenter, ScopeView, RoleViewer, "open the action center"},
		{ActionToggleZen, ScopeView, RoleViewer, "toggle zen mode"},
		{ActionQuit, ScopeView, RoleViewer, "quit"},
	}
}

// actionSpecs indexes Actions() by name for O(1) validation/lookup.
func actionSpecs() map[Action]Spec {
	out := make(map[Action]Spec, len(Actions()))
	for _, s := range Actions() {
		out[s.Action] = s
	}
	return out
}
