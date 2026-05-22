package control

import (
	"fmt"
	"sort"
)

// Keymap maps a key string (bubbletea's msg.String() form, e.g. "r", "ctrl+c")
// to an Action and owns the TUI's global bindings: the model resolves keypresses
// through Lookup, the help modal renders Help(), and blink.yaml's control.keys
// customizes it via Merge. Modal-local keys stay context-scoped in the model.
type Keymap struct {
	bindings map[string]Action
}

// DefaultKeymap is the shipped binding set. Multiple keys may map to the
// same action (q and ctrl+c both quit; / and ? both open the center).
func DefaultKeymap() Keymap {
	return Keymap{bindings: map[string]Action{
		"right":      ActionNextTab,
		"left":       ActionPrevTab,
		"[":          ActionHistBack,
		"]":          ActionHistForward,
		"tab":        ActionNextChild,
		"shift+tab":  ActionPrevChild,
		"r":          ActionRestart,
		"R":          ActionRestartAll,
		"enter":      ActionInsertBlank,
		"k":          ActionClear,
		"K":          ActionClearAll,
		"e":          ActionCursorMode,
		"up":         ActionCursorUp,
		"down":       ActionCursorDown,
		"shift+up":   ActionExtendUp,
		"shift+down": ActionExtendDown,
		" ":          ActionToggleSelect,
		"c":          ActionCopy,
		"esc":        ActionClearCursor,
		"w":          ActionWriteSelection,
		"a":          ActionAppendSelection,
		"L":          ActionToggleLogs,
		"/":          ActionCommandCenter,
		"?":          ActionCommandCenter,
		"z":          ActionToggleZen,
		"q":          ActionQuit,
		"ctrl+c":     ActionQuit,
	}}
}

// Lookup resolves a key string to its bound Action.
func (k Keymap) Lookup(key string) (Action, bool) {
	a, ok := k.bindings[key]
	return a, ok
}

// Merge applies blink.yaml control.keys overrides onto a copy of the keymap.
// Each override is key -> action-name. Binding an unknown action is an error so
// typos fail at load time; an empty action value unbinds the key.
func (k Keymap) Merge(overrides map[string]string) (Keymap, error) {
	specs := actionSpecs()
	out := make(map[string]Action, len(k.bindings)+len(overrides))
	for key, a := range k.bindings {
		out[key] = a
	}
	for key, name := range overrides {
		if name == "" {
			delete(out, key)
			continue
		}
		a := Action(name)
		if _, ok := specs[a]; !ok {
			return Keymap{}, fmt.Errorf("control.keys binds %q to unknown action %q", key, name)
		}
		out[key] = a
	}
	return Keymap{bindings: out}, nil
}

// HelpEntry is one row in the help modal: an action, the keys bound to
// it, and its catalog metadata.
type HelpEntry struct {
	Action Action
	Keys   []string
	Scope  Scope
	Help   string
}

// Help returns one entry per bound action in catalog order, each carrying its
// sorted bound keys. Unbound actions are omitted. Drives the help modal so it
// reflects live bindings.
func (k Keymap) Help() []HelpEntry {
	keysByAction := make(map[Action][]string)
	for key, a := range k.bindings {
		keysByAction[a] = append(keysByAction[a], key)
	}
	for a := range keysByAction {
		sort.Strings(keysByAction[a])
	}
	var out []HelpEntry
	for _, spec := range Actions() {
		keys, ok := keysByAction[spec.Action]
		if !ok {
			continue
		}
		out = append(out, HelpEntry{Action: spec.Action, Keys: keys, Scope: spec.Scope, Help: spec.Help})
	}
	return out
}
