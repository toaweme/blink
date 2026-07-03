package tui

import "testing"

// Test_Zen_RawTailFilters checks that in zen mode a line only streams when it
// belongs to the tabbed-to view: the all tab passes everything, a service tab
// only its own service, and a focused container only that container's lines.
func Test_Zen_RawTailFilters(t *testing.T) {
	m := NewModel([]string{"web", "docker"}, nil)
	m.rawMode = true

	// tabs = [all, web, docker]; the all tab streams every service.
	m.active = 0
	if _, ok := m.rawTail("web", "", "hi", "hi"); !ok {
		t.Fatalf("all tab dropped a web line")
	}
	if _, ok := m.rawTail("docker", "db", "db up", "db up"); !ok {
		t.Fatalf("all tab dropped a container line")
	}

	// on the web tab only web lines stream.
	m.active = 1
	if _, ok := m.rawTail("web", "", "hi", "hi"); !ok {
		t.Fatalf("web tab dropped its own line")
	}
	if _, ok := m.rawTail("docker", "db", "db up", "db up"); ok {
		t.Fatalf("web tab streamed a docker line")
	}

	// focus a container on the docker tab: only that container's lines stream,
	// and the streamed text is the bare (unprefixed) line.
	m.active = 2
	m.noteChild("docker", "db")
	m.noteChild("docker", "api")
	m.childFocus = map[string]string{"docker": "db"}
	out, ok := m.rawTail("docker", "db", "[db] db up", "db up")
	if !ok {
		t.Fatalf("focused container dropped its own line")
	}
	if out != "db up" {
		t.Fatalf("focused container streamed %q, want bare %q", out, "db up")
	}
	if _, ok := m.rawTail("docker", "api", "[api] api up", "api up"); ok {
		t.Fatalf("focused db container streamed an api line")
	}
	// a bare service-level line (no child) is filtered while a container is focused.
	if _, ok := m.rawTail("docker", "", "── running ──", ""); ok {
		t.Fatalf("focused container streamed a service-level line")
	}
}

// Test_Zen_RawNavFlushesOnChange checks that a zen-mode nav emits a flush only
// when the focused view actually moves, and no-ops otherwise.
func Test_Zen_RawNavFlushesOnChange(t *testing.T) {
	m := NewModel([]string{"web", "docker"}, nil)
	m.rawMode = true

	// a real tab move flushes.
	if cmd := m.rawNav(func() { m.gotoTab(1) }); cmd == nil {
		t.Fatalf("moving to a new tab did not flush")
	}
	// re-selecting the same tab is a no-op, no flush.
	if cmd := m.rawNav(func() { m.gotoTab(1) }); cmd != nil {
		t.Fatalf("no-op tab select still flushed")
	}
	// cycling containers on a childless tab is a no-op.
	if cmd := m.rawNav(func() { m.cycleChild(1) }); cmd != nil {
		t.Fatalf("childless container cycle still flushed")
	}
}

// Test_Zen_RawFocusLabel names the tab, or "tab · container" when focused.
func Test_Zen_RawFocusLabel(t *testing.T) {
	m := NewModel([]string{"docker"}, nil)
	m.active = 1
	if got := m.rawFocusLabel(); got != "docker" {
		t.Fatalf("label = %q, want docker", got)
	}
	m.childFocus = map[string]string{"docker": "db"}
	if got := m.rawFocusLabel(); got != "docker · db" {
		t.Fatalf("label = %q, want %q", got, "docker · db")
	}
}
