// Package control owns the TUI's bindable action catalog and keymap: the stable
// action identifiers a key can bind to, the default key bindings, and the
// blink.yaml control.keys override path. It is the contract between a keypress
// and the supervisor side effect the model dispatches.
package control

// Action is the stable identifier a key binds to.
type Action string

const (
	// ActionRestart restarts the focused service.
	ActionRestart Action = "restart"
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

// Spec describes one action: its identifier and a one-line help string. The
// catalog is the single source of truth a Keymap validates against.
type Spec struct {
	Action Action
	Help   string
}

// Actions returns the closed catalog of bindable actions. A blink.yaml
// control.keys override that names an action absent here is rejected by
// Keymap.Merge.
func Actions() []Spec {
	return []Spec{
		{ActionRestart, "restart the focused service"},
		{ActionRestartAll, "restart all services"},
		{ActionInsertBlank, "insert a blank line into the focused service's output"},
		{ActionNextTab, "next tab"},
		{ActionPrevTab, "previous tab"},
		{ActionNextChild, "focus the next container (docker tab)"},
		{ActionPrevChild, "focus the previous container (docker tab)"},
		{ActionHistBack, "back to the previously viewed tab"},
		{ActionHistForward, "forward in tab history"},
		{ActionClear, "clear the focused tab buffer"},
		{ActionClearAll, "clear all buffers"},
		{ActionCursorMode, "toggle line-export mode"},
		{ActionCursorUp, "scroll up (cursor up in cursor mode)"},
		{ActionCursorDown, "scroll down (cursor down in cursor mode)"},
		{ActionExtendUp, "extend selection up"},
		{ActionExtendDown, "extend selection down"},
		{ActionToggleSelect, "toggle the cursor line in the selection"},
		{ActionCopy, "copy selection (or cursor line) to the clipboard"},
		{ActionClearCursor, "clear selection / exit cursor mode"},
		{ActionWriteSelection, "rewrite <svc>.selected.log with the selection"},
		{ActionAppendSelection, "append the selection to <svc>.selected.log"},
		{ActionToggleLogs, "toggle log-file writing"},
		{ActionCommandCenter, "open the action center"},
		{ActionToggleZen, "toggle zen mode"},
		{ActionQuit, "quit"},
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
