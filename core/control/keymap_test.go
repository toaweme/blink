package control

import "testing"

func Test_DefaultKeymap_Lookup(t *testing.T) {
	km := DefaultKeymap()
	cases := []struct {
		key  string
		want Action
	}{
		{"r", ActionRestart},
		{"R", ActionRestartAll},
		{"q", ActionQuit},
		{"ctrl+c", ActionQuit},
		{"z", ActionToggleZen},
		{"/", ActionCommandCenter},
		{"e", ActionCursorMode},
		{"up", ActionCursorUp},
		{"down", ActionCursorDown},
		{"shift+up", ActionExtendUp},
		{"shift+down", ActionExtendDown},
		{" ", ActionToggleSelect},
		{"c", ActionCopy},
		{"esc", ActionClearCursor},
		{"w", ActionWriteSelection},
		{"L", ActionToggleLogs},
	}
	for _, tc := range cases {
		got, ok := km.Lookup(tc.key)
		if !ok || got != tc.want {
			t.Fatalf("Lookup(%q) = %q, %v; want %q", tc.key, got, ok, tc.want)
		}
	}
	if _, ok := km.Lookup("does-not-exist"); ok {
		t.Fatalf("Lookup of unbound key should report ok=false")
	}
}

func Test_Keymap_Merge_Override(t *testing.T) {
	km, err := DefaultKeymap().Merge(map[string]string{
		"x": string(ActionRestart), // bind a new key
		"r": "",                    // unbind the default
	})
	if err != nil {
		t.Fatalf("Merge returned error: %v", err)
	}
	if a, ok := km.Lookup("x"); !ok || a != ActionRestart {
		t.Fatalf("expected x -> restart, got %q ok=%v", a, ok)
	}
	if _, ok := km.Lookup("r"); ok {
		t.Fatalf("expected r to be unbound after empty override")
	}
	// the default keymap must be unchanged (Merge copies).
	if a, _ := DefaultKeymap().Lookup("r"); a != ActionRestart {
		t.Fatalf("Merge mutated the receiver keymap")
	}
}

func Test_Keymap_Merge_UnknownAction(t *testing.T) {
	_, err := DefaultKeymap().Merge(map[string]string{"x": "set-filter"})
	if err == nil {
		t.Fatalf("expected error binding an unknown action")
	}
}

// Test_DefaultKeymap_BindingsInCatalog guards against a binding referencing
// an action that isn't in the closed catalog (which Help() would then drop
// and Merge() would reject as a user override).
func Test_DefaultKeymap_BindingsInCatalog(t *testing.T) {
	specs := actionSpecs()
	for key, a := range DefaultKeymap().bindings {
		if _, ok := specs[a]; !ok {
			t.Fatalf("default binding %q -> %q is not in the action catalog", key, a)
		}
	}
}

func Test_Actions_ScopeAndRole(t *testing.T) {
	specs := actionSpecs()
	if specs[ActionRestart].Scope != ScopeSession {
		t.Fatalf("restart should be a session action")
	}
	if specs[ActionRestart].Role != RoleOperator {
		t.Fatalf("restart should require operator role")
	}
	if specs[ActionToggleZen].Scope != ScopeView {
		t.Fatalf("toggle-zen should be a view action")
	}
}
