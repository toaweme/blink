package configform

import (
	"errors"
	"strings"
	"testing"
	"time"

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
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
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
	m = send(m, keyMsg("right"))
	if m.result != resEdit {
		t.Fatalf("result = %d, want resEdit", m.result)
	}
	if m.editIdx != 1 {
		t.Fatalf("editIdx = %d, want 1", m.editIdx)
	}
}

func Test_Picker_WriteAndCancel(t *testing.T) {
	if got := send(newPicker("a"), keyMsg("enter")).result; got != resDone {
		t.Fatalf("enter result = %d, want resDone", got)
	}
	if got := send(newPicker("a"), keyMsg("esc")).result; got != resCancel {
		t.Fatalf("esc result = %d, want resCancel", got)
	}
	if got := send(newPicker("a"), keyMsg("a")).result; got != resAdd {
		t.Fatalf("a result = %d, want resAdd", got)
	}
}

func waitUntil(t *testing.T, cond func() bool) {
	t.Helper()
	for i := 0; i < 1000; i++ {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition not met within timeout")
}

func Test_ProbeManager_RunsAndStores(t *testing.T) {
	pm := newProbeManager(func(s config.Service) ([]config.Port, error) {
		if s.Name == "api" {
			return []config.Port{config.LiteralPort(8080)}, nil
		}
		return nil, nil
	})
	pm.start([]config.Service{{Name: "api"}})
	waitUntil(t, func() bool { return !pm.anyRunning() })

	running, results := pm.snapshot()
	if running["api"] {
		t.Fatal("api still marked running after completion")
	}
	out, ok := results["api"]
	if !ok || len(out.ports) != 1 || out.ports[0] != config.LiteralPort(8080) {
		t.Fatalf("results[api] = %+v, want ports [8080]", out)
	}
}

func Test_Picker_ReconcileStates(t *testing.T) {
	pm := newProbeManager(nil)
	pm.running["worker"], pm.active = true, 1
	pm.results["api"] = probeOutcome{ports: []config.Port{config.LiteralPort(8080)}}
	pm.results["db"] = probeOutcome{err: errors.New("x")}
	pm.results["empty"] = probeOutcome{}

	// buildPicker reconciles on construction.
	m := buildPicker("t", []pickItem{
		{svc: config.Service{Name: "api"}, keep: true},
		{svc: config.Service{Name: "db"}, keep: true},
		{svc: config.Service{Name: "empty"}, keep: true},
		{svc: config.Service{Name: "worker"}, keep: true},
	}, 0, false, pm)

	if m.items[0].state != probeDone || len(m.items[0].svc.Ports) != 1 {
		t.Fatalf("api state = %d ports = %v, want probeDone [8080]", m.items[0].state, m.items[0].svc.Ports)
	}
	if m.items[1].state != probeFailed {
		t.Fatalf("db state = %d, want probeFailed", m.items[1].state)
	}
	if m.items[2].state != probeNoPort {
		t.Fatalf("empty state = %d, want probeNoPort", m.items[2].state)
	}
	if m.items[3].state != probeRunning {
		t.Fatalf("worker state = %d, want probeRunning", m.items[3].state)
	}
}

func Test_Picker_ProbeKeyStartsSelectedOnly(t *testing.T) {
	release := make(chan struct{})
	pm := newProbeManager(func(config.Service) ([]config.Port, error) {
		<-release // block so we can observe the running state deterministically
		return []config.Port{config.LiteralPort(9000)}, nil
	})
	m := buildPicker("t", []pickItem{
		{svc: config.Service{Name: "api"}, keep: true},
		{svc: config.Service{Name: "off"}, keep: false},
	}, 0, false, pm)

	m = send(m, keyMsg("p"))
	if m.items[0].state != probeRunning {
		t.Fatalf("selected row state = %d, want probeRunning", m.items[0].state)
	}
	if got := pm.running["off"]; got {
		t.Fatal("unselected service should not be probed")
	}

	close(release)
	waitUntil(t, func() bool { return !pm.anyRunning() }) // let logs restore
}

func Test_Picker_View_NoPanic(t *testing.T) {
	m := buildPicker("blink init", []pickItem{
		{svc: config.Service{Name: "api", Runtime: "go", Go: &config.GoConfig{Package: ".", Args: []string{"serve"}}}, keep: true},
		{svc: config.Service{Name: "db", Runtime: "docker"}, keep: false},
	}, 0, false, nil)
	m.width = 100
	out := m.View()
	for _, want := range []string{"selected 1/2 services", "SERVICE", "RUNTIME", "COMMAND", "PORTS", "go run . serve"} {
		if !strings.Contains(out, want) {
			t.Fatalf("View() missing %q in:\n%s", want, out)
		}
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
