package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// cursor model: a single-line park on the active tab. ↑/↓ moves it,
// esc clears it, click jumps it. Multi-line selection is the selection
// dialog's job - open it with enter and grow the range there.

// ensureCursor seeds a cursor for the active tab if none exists. The
// fresh state parks the cursor on the tail line so it doesn't surprise
// the user by parking on the top.
func (m *Model) ensureCursor() {
	if m.tabCursor == nil {
		m.tabCursor = map[string]int{}
	}
	tab := m.activeTab()
	if _, ok := m.tabCursor[tab]; ok {
		return
	}
	buf := m.buffers[tab]
	idx := 0
	if n := len(buf); n > 0 {
		idx = n - 1
	}
	m.tabCursor[tab] = idx
}

// cursorAt returns the current line index for the active tab, or -1
// if the tab has no cursor yet.
func (m Model) cursorAt() int {
	if m.tabCursor == nil {
		return -1
	}
	idx, ok := m.tabCursor[m.activeTab()]
	if !ok {
		return -1
	}
	return idx
}

// moveCursor walks the cursor by delta lines. Positive delta = down.
func (m *Model) moveCursor(delta int) {
	m.ensureCursor()
	tab := m.activeTab()
	buf := m.buffers[tab]
	if len(buf) == 0 {
		return
	}
	idx := m.tabCursor[tab] + delta
	if idx < 0 {
		idx = 0
	}
	if idx > len(buf)-1 {
		idx = len(buf) - 1
	}
	m.tabCursor[tab] = idx
	m.scrollCursorIntoView()
}

// clearCursor drops the cursor on the active tab.
func (m *Model) clearCursor() {
	if m.tabCursor == nil {
		return
	}
	delete(m.tabCursor, m.activeTab())
}

// jumpCursorTo moves the cursor to idx. Used by mouse click.
func (m *Model) jumpCursorTo(idx int) {
	m.ensureCursor()
	tab := m.activeTab()
	buf := m.buffers[tab]
	if len(buf) == 0 {
		return
	}
	if idx < 0 {
		idx = 0
	}
	if idx > len(buf)-1 {
		idx = len(buf) - 1
	}
	m.tabCursor[tab] = idx
	m.scrollCursorIntoView()
}

// scrollCursorIntoView snaps the viewport so the cursor row stays
// visible after a moveCursor. Uses the wrappedToBuffer mapping built by
// the last render, so callers should refreshViewport first.
func (m *Model) scrollCursorIntoView() {
	if !m.ready {
		return
	}
	idx := m.cursorAt()
	if idx < 0 || len(m.wrappedToBuffer) == 0 {
		return
	}
	row := -1
	for i, b := range m.wrappedToBuffer {
		if b == idx {
			row = i
			break
		}
	}
	if row < 0 {
		return
	}
	top := m.vp.YOffset
	bot := top + m.vp.Height - 1
	if row < top {
		m.vp.SetYOffset(row)
	} else if row > bot {
		m.vp.SetYOffset(row - m.vp.Height + 1)
	}
	m.followTail = m.vp.AtBottom()
}

// toggleCursorMode enters/exits cursor mode. Entering anchors the cursor on a
// currently visible line (so it tracks where the user scrolled to, not a stale
// parked position) and scrolls it into view; exiting clears the selection so it
// doesn't linger invisibly.
func (m *Model) toggleCursorMode() {
	m.cursorMode = !m.cursorMode
	if m.cursorMode {
		m.ensureCursor()
		m.clampCursorToViewport()
		m.scrollCursorIntoView()
		return
	}
	m.clearSelection()
}

// clampCursorToViewport pulls the cursor onto a line that is currently on
// screen. Used only when entering cursor mode: after scrolling away in scroll
// mode the parked cursor can sit far outside the viewport, and seeding the
// cursor there would yank the view back to that stale line. Snapping it to the
// nearest visible line keeps the cursor where the user is actually looking. It
// is deliberately NOT called on per-line moves - doing so makes a single ↑/↓
// jump by two lines (snap + move).
func (m *Model) clampCursorToViewport() {
	if !m.ready || len(m.wrappedToBuffer) == 0 {
		return
	}
	idx := m.cursorAt()
	if idx < 0 {
		return
	}
	cursorRow := -1
	for row, b := range m.wrappedToBuffer {
		if b == idx {
			cursorRow = row
			break
		}
	}
	if cursorRow < 0 {
		return
	}
	top := m.vp.YOffset
	if top < 0 {
		top = 0
	}
	bot := top + m.vp.Height - 1
	if last := len(m.wrappedToBuffer) - 1; bot > last {
		bot = last
	}
	switch {
	case cursorRow < top:
		m.tabCursor[m.activeTab()] = m.wrappedToBuffer[top]
	case cursorRow > bot:
		m.tabCursor[m.activeTab()] = m.wrappedToBuffer[bot]
	}
}

// escapeCursor is the esc key: collapse the selection first, then exit cursor
// mode on a second press.
func (m *Model) escapeCursor() {
	if m.hasSelection() {
		m.clearSelection()
		return
	}
	m.cursorMode = false
}

// enterCursorMode lazily turns cursor mode on, used by the selection keys so
// they work straight from scroll mode.
func (m *Model) enterCursorMode() {
	if !m.cursorMode {
		m.cursorMode = true
		m.ensureCursor()
		m.clampCursorToViewport()
	}
}

func (m Model) hasSelection() bool { return len(m.selected[m.activeTab()]) > 0 }

func (m *Model) clearSelection() {
	delete(m.selected, m.activeTab())
}

// setSelected adds or removes idx from the active tab's selection set.
func (m *Model) setSelected(idx int, on bool) {
	tab := m.activeTab()
	set := m.selected[tab]
	if set == nil {
		set = map[int]bool{}
		m.selected[tab] = set
	}
	if on {
		set[idx] = true
	} else {
		delete(set, idx)
	}
}

// toggleSelect flips the cursor line in/out of the selection (space).
func (m *Model) toggleSelect() {
	m.enterCursorMode()
	idx := m.cursorAt()
	if idx < 0 {
		return
	}
	m.setSelected(idx, !m.selected[m.activeTab()][idx])
}

// extendSelection grows or shrinks the selection with shift+↑/↓. It is
// stateless - there is no stored anchor or range, only the current set of
// selected rows. Each press flips exactly one line: the selection trails the
// cursor. Moving onto a fresh (unselected) line is extending, so the line the
// cursor leaves is selected and the cursor advances onto an unselected line.
// Moving onto an already-selected line is retreating, so that line is dropped
// as the cursor steps back onto it. Bound to shift+↑/↓.
func (m *Model) extendSelection(delta int) {
	m.enterCursorMode()
	tab := m.activeTab()
	c := m.cursorAt()
	if c < 0 {
		return
	}
	dest := c + delta
	if dest < 0 || dest > len(m.buffers[tab])-1 {
		return // at a buffer edge: no line to move onto
	}
	if m.selected[tab][dest] {
		// retreating: drop the row we step back onto.
		m.setSelected(dest, false)
	} else {
		// extending: select the row we leave; the cursor advances onto a fresh row.
		m.setSelected(c, true)
	}
	m.moveCursor(delta) // lands on dest and scrolls it into view
}

// selectionIndices returns the active tab's selected buffer indices, sorted.
func (m Model) selectionIndices() []int {
	set := m.selected[m.activeTab()]
	if len(set) == 0 {
		return nil
	}
	idxs := make([]int, 0, len(set))
	for i := range set {
		idxs = append(idxs, i)
	}
	sort.Ints(idxs)
	return idxs
}

// linesAt returns the stripped text of the given buffer indices on the active
// tab, skipping any out-of-range index.
func (m Model) linesAt(idxs []int) []string {
	buf := m.buffers[m.activeTab()]
	out := make([]string, 0, len(idxs))
	for _, i := range idxs {
		if i < 0 || i >= len(buf) {
			continue
		}
		out = append(out, ansi.Strip(buf[i]))
	}
	return out
}

// targetIndices resolves what a copy/write acts on: the explicit selection,
// or the cursor line when nothing is selected (cursor mode only).
func (m *Model) targetIndices() []int {
	if idxs := m.selectionIndices(); len(idxs) > 0 {
		return idxs
	}
	if m.cursorMode {
		if c := m.cursorAt(); c >= 0 {
			return []int{c}
		}
	}
	return nil
}

// copySelection copies the target lines to the system clipboard.
func (m *Model) copySelection() {
	lines := m.linesAt(m.targetIndices())
	if len(lines) == 0 {
		return
	}
	if err := clipboard.WriteAll(strings.Join(lines, "\n")); err != nil {
		m.appendLine(m.activeTab(), m.feedbackErr("copy: "+err.Error()))
		return
	}
	m.setFlash(fmt.Sprintf("COPIED %d", len(lines)), "82")
}

// writeSelection (bound to `w`) replaces <logDir>/<tab>.selected.log with the
// current selection, and appendSelection (bound to `a`) adds the selection to
// whatever is already there. `w` is the common case: the file is a fresh
// handoff of "here's the evidence right now" - for a coding agent reading it
// locally, or as the clipboard copy a remote peer pastes into chat. `a` is the
// opt-in accumulator for assembling several captures across one investigation.
//
// Both keep the selection and cursor afterwards so a wrong pick is a quick
// fix: toggle the offending line and press `w` again (rewrite is idempotent),
// rather than re-selecting from scratch. Clear it explicitly with esc.
func (m *Model) writeSelection()  { m.emitSelection(false) }
func (m *Model) appendSelection() { m.emitSelection(true) }

func (m *Model) emitSelection(appendMode bool) {
	lines := m.linesAt(m.targetIndices())
	if len(lines) == 0 {
		return
	}
	tab := m.activeTab()
	if m.logDir != "" {
		if err := writeSelectedLog(m.logDir, tab, lines, appendMode); err != nil {
			m.appendLine(tab, m.feedbackErr("write: "+err.Error()))
		} else {
			verb := "WRITTEN"
			if appendMode {
				verb = "APPENDED"
			}
			m.setFlash(fmt.Sprintf("%s %d", verb, len(lines)), "44")
		}
	}
	// clipboard is a bonus (and the only sink for a remote mirror with no
	// logDir); the file write is the contract.
	_ = clipboard.WriteAll(strings.Join(lines, "\n"))
}

func (m Model) feedbackOK(s string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Render(s)
}

func (m Model) feedbackErr(s string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Render(s)
}

// writeSelectedLog writes lines to <logDir>/<tab>.selected.log, creating the
// directory and file as needed. In rewrite mode the file is truncated to hold
// exactly the selection (clean evidence for whoever reads it). In append mode
// the lines are added after a one-line header so successive captures stay
// visually separable.
func writeSelectedLog(logDir, tab string, lines []string, appendMode bool) error {
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return fmt.Errorf("failed to create log dir %q: %w", logDir, err)
	}
	path := filepath.Join(logDir, tab+".selected.log")
	flags := os.O_CREATE | os.O_WRONLY
	body := strings.Join(lines, "\n") + "\n"
	if appendMode {
		flags |= os.O_APPEND
		body = fmt.Sprintf("# --- selection: %d line(s) ---\n", len(lines)) + body
	} else {
		flags |= os.O_TRUNC
	}
	f, err := os.OpenFile(path, flags, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open %q: %w", path, err)
	}
	defer f.Close()
	if _, err := f.WriteString(body); err != nil {
		return fmt.Errorf("failed to write %q: %w", path, err)
	}
	return nil
}
