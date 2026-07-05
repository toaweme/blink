package tui

import "testing"

// feedDocker builds a model whose active tab is a managed "docker" service and
// streams one line per (container, text) pair through the line handler, so the
// child buffers and ring populate exactly as they would at runtime.
func feedDocker(t *testing.T, lines []LineMsg) *Model {
	t.Helper()
	m := NewModel([]string{"docker"}, nil)
	m.active = 0 // lone service: no all-tab, tabs = [docker]
	if m.activeTab() != "docker" {
		t.Fatalf("active tab = %q, want docker", m.activeTab())
	}
	for _, ln := range lines {
		next, _ := m.handleLineMsg(ln)
		m = next.(*Model)
	}
	return m
}

func Test_Container_NoteAndBuffers(t *testing.T) {
	m := feedDocker(t, []LineMsg{
		{Service: "docker", Child: "db", Line: "db up"},
		{Service: "docker", Child: "api", Line: "api up"},
		{Service: "docker", Child: "db", Line: "db ready"},
	})

	// children recorded in first-seen order, deduped.
	got := m.childList["docker"]
	if len(got) != 2 || got[0] != "db" || got[1] != "api" {
		t.Fatalf("childList = %v, want [db api]", got)
	}

	// per-container buffers hold the raw (unprefixed) lines.
	db := m.buffers["docker"+childSep+"db"]
	if len(db) != 2 || db[0] != "db up" || db[1] != "db ready" {
		t.Fatalf("db buffer = %v, want [db up, db ready]", db)
	}
	api := m.buffers["docker"+childSep+"api"]
	if len(api) != 1 || api[0] != "api up" {
		t.Fatalf("api buffer = %v, want [api up]", api)
	}

	// the merged service buffer keeps every container's line (3 total).
	if n := len(m.buffers["docker"]); n != 3 {
		t.Fatalf("merged buffer has %d lines, want 3", n)
	}
}

func Test_Container_CycleFocus(t *testing.T) {
	m := feedDocker(t, []LineMsg{
		{Service: "docker", Child: "db", Line: "db up"},
		{Service: "docker", Child: "api", Line: "api up"},
	})

	// default: merged view, viewKey is the bare tab.
	if got := m.viewKey(); got != "docker" {
		t.Fatalf("default viewKey = %q, want docker", got)
	}

	// forward ring: all -> db -> api -> all.
	want := []string{"db", "api", ""}
	for i, w := range want {
		m.cycleChild(1)
		if got := m.childFocus["docker"]; got != w {
			t.Fatalf("after %d forward cycles focus = %q, want %q", i+1, got, w)
		}
	}

	// backward from "all" wraps to the last container.
	m.cycleChild(-1)
	if got := m.childFocus["docker"]; got != "api" {
		t.Fatalf("backward cycle focus = %q, want api", got)
	}
	if got := m.viewKey(); got != "docker"+childSep+"api" {
		t.Fatalf("focused viewKey = %q, want docker<sep>api", got)
	}
}

func Test_TabHistory_BackForward(t *testing.T) {
	// tabs = [all, a, b, c, d]; start on all (index 0).
	m := NewModel([]string{"a", "b", "c", "d"}, nil)

	m.gotoTab(1) // jump to "a"
	m.gotoTab(4) // jump to "d"
	if m.active != 4 {
		t.Fatalf("active = %d, want 4", m.active)
	}

	// back walks the trail: d -> a -> all.
	m.histBack()
	if m.active != 1 {
		t.Fatalf("after back active = %d, want 1 (a)", m.active)
	}
	m.histBack()
	if m.active != 0 {
		t.Fatalf("after second back active = %d, want 0 (all)", m.active)
	}
	// at the start of the trail, back is a no-op.
	m.histBack()
	if m.active != 0 {
		t.Fatalf("back past start moved to %d, want 0", m.active)
	}

	// forward replays: all -> a -> d.
	m.histForward()
	m.histForward()
	if m.active != 4 {
		t.Fatalf("after two forward active = %d, want 4 (d)", m.active)
	}

	// a fresh jump from the middle forks the trail (forward branch dropped).
	m.histBack() // -> a (index 1)
	m.gotoTab(2) // jump to "b": forward entry (d) is discarded
	m.histForward()
	if m.active != 2 {
		t.Fatalf("forward after fork moved to %d, want 2 (b, no-op)", m.active)
	}
	m.histBack()
	if m.active != 1 {
		t.Fatalf("back after fork active = %d, want 1 (a)", m.active)
	}
}

func Test_Container_CycleNoopWithoutChildren(t *testing.T) {
	m := NewModel([]string{"web"}, nil)
	m.active = 0 // lone service: no all-tab, plain shell service, no children
	m.cycleChild(1)
	if got := m.childFocus["web"]; got != "" {
		t.Fatalf("cycle on childless tab set focus %q, want empty", got)
	}
	if got := m.viewKey(); got != "web" {
		t.Fatalf("viewKey = %q, want web", got)
	}
}
