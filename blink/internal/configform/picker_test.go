package configform

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/toaweme/blink/core/config"
)

func newPicker(names ...string) picker {
	items := make([]pickItem, len(names))
	for i, n := range names {
		items[i] = pickItem{svc: config.Service{Name: n}, keep: true}
	}
	return picker{title: "t", items: items}
}

func send(m picker, msg tea.Msg) picker {
	out, _ := m.Update(msg)
	return out.(picker)
}

func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "space":
		return tea.KeyMsg{Type: tea.KeySpace}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func Test_Picker_NavigateAndToggle(t *testing.T) {
	m := newPicker("a", "b", "c")

	m = send(m, keyMsg("down"))
	if m.cursor != 1 {
		t.Fatalf("cursor = %d, want 1", m.cursor)
	}
	// space drops "b".
	m = send(m, keyMsg("space"))
	if m.items[1].keep {
		t.Fatalf("item b still kept after space")
	}
	// can't move past the ends.
	m = send(m, keyMsg("up"))
	m = send(m, keyMsg("up"))
	if m.cursor != 0 {
		t.Fatalf("cursor = %d, want clamped to 0", m.cursor)
	}
}

func Test_Picker_EditResult(t *testing.T) {
	m := newPicker("a", "b")
	m = send(m, keyMsg("down"))
	m = send(m, keyMsg("enter"))
	if m.result != resEdit {
		t.Fatalf("result = %d, want resEdit", m.result)
	}
	if m.editIdx != 1 {
		t.Fatalf("editIdx = %d, want 1", m.editIdx)
	}
}

func Test_Picker_WriteAndCancel(t *testing.T) {
	if got := send(newPicker("a"), keyMsg("w")).result; got != resDone {
		t.Fatalf("w result = %d, want resDone", got)
	}
	if got := send(newPicker("a"), keyMsg("q")).result; got != resCancel {
		t.Fatalf("q result = %d, want resCancel", got)
	}
	if got := send(newPicker("a"), keyMsg("a")).result; got != resAdd {
		t.Fatalf("a result = %d, want resAdd", got)
	}
}

func Test_Picker_DetectGatedByAllow(t *testing.T) {
	// allowDetect off: `d` is inert, so the program does not quit and result
	// stays at its initial value (no action requested).
	off := send(newPicker("a"), keyMsg("d"))
	if off.result != resCancel { // resCancel is the zero value
		t.Fatalf("d with allowDetect=false set result to %d, want unchanged", off.result)
	}
	m := newPicker("a")
	m.allowDetect = true
	if got := send(m, keyMsg("d")).result; got != resDetect {
		t.Fatalf("d result = %d, want resDetect", got)
	}
}
