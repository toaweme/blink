package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seed builds a model with one service tab carrying n lines and the active
// tab set to that service.
func seed(t *testing.T, n int) Model {
	t.Helper()
	m := NewModel([]string{"web"}, nil)
	m.active = 1 // tabs = [all, web]
	if m.activeTab() != "web" {
		t.Fatalf("active tab = %q, want web", m.activeTab())
	}
	for i := 0; i < n; i++ {
		m.appendLine("web", "line"+itoa(i))
	}
	return m
}

func itoa(i int) string { return string(rune('0' + i)) }

func Test_Selection_GapsAndWrite(t *testing.T) {
	m := seed(t, 5)
	m.logDir = t.TempDir()

	// enter cursor mode: cursor parks on the tail line (idx 4).
	m.toggleCursorMode()
	if !m.cursorMode {
		t.Fatalf("toggleCursorMode did not enable cursor mode")
	}
	if got := m.cursorAt(); got != 4 {
		t.Fatalf("cursor seeded at %d, want tail 4", got)
	}

	// select tail, jump up two lines, select again -> a gap at index 3.
	m.toggleSelect()
	m.moveCursor(-2) // now on idx 2
	m.toggleSelect()

	idxs := m.selectionIndices()
	if len(idxs) != 2 || idxs[0] != 2 || idxs[1] != 4 {
		t.Fatalf("selection = %v, want [2 4]", idxs)
	}

	m.writeSelection()

	data, err := os.ReadFile(filepath.Join(m.logDir, "web.selected.log"))
	if err != nil {
		t.Fatalf("reading selected log: %v", err)
	}
	out := string(data)
	if !strings.Contains(out, "line2") || !strings.Contains(out, "line4") {
		t.Fatalf("selected log missing selected lines: %q", out)
	}
	if strings.Contains(out, "line3") {
		t.Fatalf("selected log leaked the gap line: %q", out)
	}
	// writing keeps the selection so a wrong pick is a quick fix-and-rewrite.
	if !m.hasSelection() {
		t.Fatalf("writeSelection should keep the selection for quick fixing")
	}
	if got := m.cursorAt(); got != 2 {
		t.Fatalf("writeSelection moved the cursor to %d, want it kept at 2", got)
	}
}

// Test_WriteSelection_Rewrites verifies that `w` truncates the file: a second
// write with a different selection replaces the first rather than stacking.
func Test_WriteSelection_Rewrites(t *testing.T) {
	m := seed(t, 5)
	m.logDir = t.TempDir()
	path := filepath.Join(m.logDir, "web.selected.log")

	m.toggleCursorMode() // cursor at 4
	m.toggleSelect()     // select line4
	m.writeSelection()

	// fix the pick: drop line4, select line0 instead, rewrite.
	m.toggleSelect() // deselect line4
	m.moveCursor(-4) // cursor at 0
	m.toggleSelect() // select line0
	m.writeSelection()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading selected log: %v", err)
	}
	out := string(data)
	if !strings.Contains(out, "line0") {
		t.Fatalf("rewrite missing the new selection: %q", out)
	}
	if strings.Contains(out, "line4") {
		t.Fatalf("rewrite should have dropped the prior selection: %q", out)
	}
}

// Test_AppendSelection_Accumulates verifies that `a` keeps prior captures: two
// appends with different selections both survive in the file.
func Test_AppendSelection_Accumulates(t *testing.T) {
	m := seed(t, 5)
	m.logDir = t.TempDir()
	path := filepath.Join(m.logDir, "web.selected.log")

	m.toggleCursorMode() // cursor at 4
	m.toggleSelect()     // select line4
	m.appendSelection()

	m.toggleSelect() // deselect line4
	m.moveCursor(-4) // cursor at 0
	m.toggleSelect() // select line0
	m.appendSelection()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading selected log: %v", err)
	}
	out := string(data)
	if !strings.Contains(out, "line0") || !strings.Contains(out, "line4") {
		t.Fatalf("append should keep both captures: %q", out)
	}
}

// Test_Extend_ZigZagStateless walks the cursor up and down across its starting
// line and asserts the selection always equals the contiguous run the cursor
// has covered - no stale rows left selected when retreating. The logic is
// stateless (no anchor), so this holds for any path.
func Test_Extend_ZigZagStateless(t *testing.T) {
	m := seed(t, 6) // lines 0..5
	m.toggleCursorMode()
	m.moveCursor(-3) // cursor at 2

	// the selection trails the cursor by one: each move selects the line left
	// behind, the cursor advances onto an unselected line, and retreating drops
	// the line stepped back onto.
	type step struct {
		delta int
		want  []int
	}
	steps := []step{
		{+1, []int{2}},    // leave 2 selected, cursor -> 3
		{+1, []int{2, 3}}, // leave 3, cursor -> 4
		{-1, []int{2}},    // step back onto 3, drop it, cursor -> 3
		{-1, []int{}},     // step back onto 2, drop it, cursor -> 2
		{-1, []int{2}},    // reverse: leave 2, cursor -> 1
		{+1, []int{}},     // step back onto 2, drop it, cursor -> 2
		{+1, []int{2}},    // leave 2 again, cursor -> 3
	}
	for i, s := range steps {
		m.extendSelection(s.delta)
		got := m.selectionIndices()
		if len(got) != len(s.want) {
			t.Fatalf("step %d (delta %+d): selection = %v, want %v", i, s.delta, got, s.want)
		}
		for j := range got {
			if got[j] != s.want[j] {
				t.Fatalf("step %d (delta %+d): selection = %v, want %v", i, s.delta, got, s.want)
			}
		}
	}
}

func Test_Selection_ExtendContiguous(t *testing.T) {
	m := seed(t, 5)
	m.toggleCursorMode()  // cursor at 4
	m.extendSelection(-1) // leaves 4 selected, cursor -> 3
	m.extendSelection(-1) // leaves 3 selected, cursor -> 2
	// the selection trails the cursor: two moves up from 4 select 4 and 3, with
	// the cursor now resting on the still-unselected line 2.
	idxs := m.selectionIndices()
	if len(idxs) != 2 || idxs[0] != 3 || idxs[1] != 4 {
		t.Fatalf("contiguous extend = %v, want [3 4]", idxs)
	}
	if got := m.cursorAt(); got != 2 {
		t.Fatalf("cursor at %d, want 2 (one ahead of the selection)", got)
	}
}

func Test_Selection_ExtendRetreatDeselects(t *testing.T) {
	m := seed(t, 5)
	m.toggleCursorMode()  // cursor at 4
	m.extendSelection(-1) // {4}, cursor -> 3
	m.extendSelection(-1) // {3,4}, cursor -> 2

	m.extendSelection(1) // step back onto 3, drop it -> {4}, cursor -> 3
	if idxs := m.selectionIndices(); len(idxs) != 1 || idxs[0] != 4 {
		t.Fatalf("after one retreat = %v, want [4]", idxs)
	}

	m.extendSelection(1) // step back onto 4, drop it -> {}, cursor -> 4
	if idxs := m.selectionIndices(); len(idxs) != 0 {
		t.Fatalf("after full retreat = %v, want []", idxs)
	}
}

func Test_Escape_ClearsThenExits(t *testing.T) {
	m := seed(t, 3)
	m.toggleCursorMode()
	m.toggleSelect()
	// first esc clears the selection but stays in cursor mode.
	m.escapeCursor()
	if m.hasSelection() {
		t.Fatalf("first esc should clear the selection")
	}
	if !m.cursorMode {
		t.Fatalf("first esc should not leave cursor mode while a selection existed")
	}
	// second esc exits cursor mode.
	m.escapeCursor()
	if m.cursorMode {
		t.Fatalf("second esc should exit cursor mode")
	}
}

func Test_WriteSelection_NoSelectionNoCursorMode(t *testing.T) {
	m := seed(t, 3)
	m.logDir = t.TempDir()
	// not in cursor mode and nothing selected: writing is a no-op.
	m.writeSelection()
	if _, err := os.Stat(filepath.Join(m.logDir, "web.selected.log")); !os.IsNotExist(err) {
		t.Fatalf("expected no file written, got err=%v", err)
	}
}
