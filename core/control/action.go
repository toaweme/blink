package control

// Scope separates the two kinds of action the TUI can bind a key to.
type Scope int

const (
	// ScopeSession actions act on the supervised services. They are role-gated
	// and wire-encodable, dispatching identically from the local TUI or a remote
	// mirror, and reuse the wire Verb strings.
	ScopeSession Scope = iota
	// ScopeView actions are consumer-local presentation concerns (scroll, switch
	// tab, toggle zen, quit) and never cross the wire.
	ScopeView
)

// Action is the stable identifier a key binds to. Session actions reuse the
// wire Verb* strings so a binding and a command share one name.
type Action string

const (
	// ActionRestart restarts the focused service. Only key-bound actions live
	// here; other verbs (list, signal, send, dump-logs, resync) are issued over
	// the wire directly and have no keybinding.
	ActionRestart = Action(VerbRestart)
	// ActionRestartAll restarts every service.
	ActionRestartAll Action = "restart-all"
	// ActionInsertBlank publishes a blank line into the focused service's output
	// stream (buffer and log file), mutating the shared Hub output.
	ActionInsertBlank Action = "insert-blank"

	// ActionQuit quits the consumer.
	ActionQuit Action = "quit"
	// ActionCommandCenter opens the action center.
	ActionCommandCenter Action = "command-center"
	// ActionToggleZen toggles zen mode.
	ActionToggleZen Action = "toggle-zen"
	// ActionToggleLogs toggles log-file writing.
	ActionToggleLogs Action = "toggle-logs"
	// ActionNextTab moves focus to the next tab.
	ActionNextTab Action = "next-tab"
	// ActionPrevTab moves focus to the previous tab.
	ActionPrevTab Action = "prev-tab"
	// ActionNextChild focuses the next child of a runtime-managed service
	// (docker compose containers) on the active tab. No-op on tabs without
	// children.
	ActionNextChild Action = "next-child"
	// ActionPrevChild focuses the previous child of a runtime-managed service.
	ActionPrevChild Action = "prev-child"
	// ActionHistBack walks back along the visited-tab trail (browser-style),
	// distinct from next/prev-tab which step to the adjacent tab.
	ActionHistBack Action = "hist-back"
	// ActionHistForward walks forward along the visited-tab trail.
	ActionHistForward Action = "hist-forward"
	// ActionClear clears the focused tab buffer.
	ActionClear Action = "clear"
	// ActionClearAll clears every buffer.
	ActionClearAll Action = "clear-all"
	// ActionCursorMode toggles line cursor mode. While off, cursor-up/down
	// scroll the viewport; while on they move the cursor and selection keys are
	// live.
	ActionCursorMode Action = "cursor-mode"
	// ActionCursorUp scrolls up, or moves the cursor up in cursor mode.
	ActionCursorUp Action = "cursor-up"
	// ActionCursorDown scrolls down, or moves the cursor down in cursor mode.
	ActionCursorDown Action = "cursor-down"
	// ActionExtendUp extends the selection up.
	ActionExtendUp Action = "extend-up"
	// ActionExtendDown extends the selection down.
	ActionExtendDown Action = "extend-down"
	// ActionToggleSelect toggles the cursor line in the selection.
	ActionToggleSelect Action = "toggle-select"
	// ActionCopy copies the selection (or cursor line) to the clipboard.
	ActionCopy Action = "copy"
	// ActionClearCursor clears the selection and exits cursor mode.
	ActionClearCursor Action = "clear-cursor"
	// ActionWriteSelection rewrites <svc>.selected.log with the selection.
	ActionWriteSelection Action = "write-selection"
	// ActionAppendSelection appends the selection to <svc>.selected.log.
	ActionAppendSelection Action = "append-selection"
)

// Spec describes one action: its scope, the minimum role to dispatch it
// (session actions only), and a one-line help string. The catalog is the single
// source of truth a Keymap validates against.
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
