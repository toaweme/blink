package tui

import "testing"

// Test_LinesMsg_IngestsWholeBurst checks that a coalesced burst lands every line
// in the right buffers - the merged service view, the all-tab, and per-container
// buffers - exactly as a sequence of single LineMsgs would, but in one pass.
func Test_LinesMsg_IngestsWholeBurst(t *testing.T) {
	// two services so the aggregate all-tab exists (a lone service has none).
	m := NewModel([]string{"web", "docker"}, nil)

	burst := LinesMsg{Lines: []LineMsg{
		{Service: "docker", Child: "db", Line: "db 1"},
		{Service: "docker", Child: "api", Line: "api 1"},
		{Service: "docker", Child: "db", Line: "db 2"},
	}}
	next, _ := m.handleLinesMsg(burst)
	m = next.(*Model)

	if got := m.childList["docker"]; len(got) != 2 || got[0] != "db" || got[1] != "api" {
		t.Fatalf("childList = %v, want [db api]", got)
	}
	if db := m.buffers["docker"+childSep+"db"]; len(db) != 2 || db[0] != "db 1" || db[1] != "db 2" {
		t.Fatalf("db buffer = %v, want [db 1, db 2]", db)
	}
	if n := len(m.buffers["docker"]); n != 3 {
		t.Fatalf("merged buffer has %d lines, want 3", n)
	}
	if n := len(m.buffers[allTab]); n != 3 {
		t.Fatalf("all-tab buffer has %d lines, want 3", n)
	}
}

// Test_LinesMsg_ZenBuffersAll checks that zen (chromeless) mode does not change
// ingestion: a burst still lands every line in its buffer, since zen only hides
// the chrome and never filters output.
func Test_LinesMsg_ZenBuffersAll(t *testing.T) {
	m := NewModel([]string{"web", "docker"}, nil)
	m.chromeless = true
	m.active = 1 // "web" service tab

	burst := LinesMsg{Lines: []LineMsg{
		{Service: "web", Line: "web up"},
		{Service: "docker", Child: "db", Line: "db up"},
		{Service: "web", Line: "web ready"},
	}}
	m.handleLinesMsg(burst)

	if web := m.buffers["web"]; len(web) != 2 {
		t.Fatalf("web buffer = %v, want 2 lines", web)
	}
	if db := m.buffers["docker"+childSep+"db"]; len(db) != 1 {
		t.Fatalf("db buffer = %v, want 1 line", db)
	}
}
