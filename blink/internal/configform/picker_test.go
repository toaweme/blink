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
		items[i] = pickItem{svc: config.Service{Name: n}, enabled: true}
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
	// space disables "b" (kept, not dropped).
	m = send(m, keyMsg("space"))
	if m.items[1].enabled {
		t.Fatalf("item b still enabled after space")
	}
	if len(m.items) != 3 {
		t.Fatalf("space dropped a row; deselect must keep it (%d rows)", len(m.items))
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

func Test_Picker_DeselectDisablesNotDrops(t *testing.T) {
	m := newPicker("a", "b")
	// space deselects "a": that flips enabled, never removes the row, so the
	// service is retained and written back disabled.
	m = send(m, keyMsg("space"))
	if m.items[0].enabled {
		t.Fatalf("deselected service should be disabled (enabled=false)")
	}
	if len(m.items) != 2 {
		t.Fatalf("deselect dropped a row (%d left); it must keep the service", len(m.items))
	}

	out := collectServices(m.items)
	if len(out) != 2 {
		t.Fatalf("collectServices dropped a service: got %d, want 2", len(out))
	}
	if !out[0].Disabled {
		t.Fatalf("service a should be written with Disabled set")
	}
	if out[1].Disabled {
		t.Fatalf("service b should stay enabled")
	}
}

func Test_Picker_RemoveDropsService(t *testing.T) {
	m := newPicker("a", "b", "c")
	m = send(m, keyMsg("down")) // cursor on "b"
	m = send(m, keyMsg("x"))    // remove "b"
	if len(m.items) != 2 || m.items[0].svc.Name != "a" || m.items[1].svc.Name != "c" {
		t.Fatalf("remove did not drop b: %+v", m.items)
	}
	// removing the last row clamps the cursor so it never dangles past the slice.
	m = send(m, keyMsg("down")) // cursor on "c" (idx 1)
	m = send(m, keyMsg("x"))    // remove "c"
	if m.cursor != 0 || len(m.items) != 1 {
		t.Fatalf("cursor=%d len=%d after removing last row, want 0/1", m.cursor, len(m.items))
	}
}

func Test_CollectServices_DisabledRoundTrip(t *testing.T) {
	// a service loaded already-disabled stays disabled; re-enabling it clears the
	// flag. Both survive collection (removal is the only way to drop one).
	items := []pickItem{
		{svc: config.Service{Name: "a"}, enabled: true},
		{svc: config.Service{Name: "b", Disabled: true}, enabled: false},
	}
	out := collectServices(items)
	if len(out) != 2 {
		t.Fatalf("collect = %+v, want both a and b", out)
	}
	if out[0].Disabled || !out[1].Disabled {
		t.Fatalf("collect disabled flags = [%v %v], want [false true]", out[0].Disabled, out[1].Disabled)
	}
}

func waitUntil(t *testing.T, cond func() bool) {
	t.Helper()
	for range 1000 {
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
		{svc: config.Service{Name: "api"}, enabled: true},
		{svc: config.Service{Name: "db"}, enabled: true},
		{svc: config.Service{Name: "empty"}, enabled: true},
		{svc: config.Service{Name: "worker"}, enabled: true},
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

func Test_Picker_EditedPortsSurviveReconcile(t *testing.T) {
	pm := newProbeManager(nil)
	pm.results["web"] = probeOutcome{ports: []config.Port{
		config.LiteralPort(3000), config.LiteralPort(4206), config.LiteralPort(56260),
	}}

	// the user probed "web" (3 ports), then edited it down to one; the stored
	// probe result must not clobber that edit when the picker is rebuilt.
	// buildPicker reconciles on construction, which is exactly that rebuild.
	m := buildPicker("t", []pickItem{
		{svc: config.Service{Name: "web", Ports: []config.Port{config.LiteralPort(3000)}}, enabled: true, edited: true},
	}, 0, false, pm)

	if got := m.items[0].svc.Ports; len(got) != 1 || got[0] != config.LiteralPort(3000) {
		t.Fatalf("edited ports = %v, want [3000] preserved", got)
	}
	if m.items[0].state != probeDone {
		t.Fatalf("state = %d, want probeDone", m.items[0].state)
	}
}

func Test_Picker_ReprobeClearsEditedAndOverwrites(t *testing.T) {
	pm := newProbeManager(func(config.Service) ([]config.Port, error) {
		return []config.Port{config.LiteralPort(3000), config.LiteralPort(4206)}, nil
	})
	// a hand-edited service (edited=true) whose ports the user trimmed.
	m := buildPicker("t", []pickItem{
		{svc: config.Service{Name: "web", Ports: []config.Port{config.LiteralPort(3000)}}, enabled: true, edited: true},
	}, 0, false, pm)

	// pressing `p` is an explicit opt-in: it must clear edited so the fresh
	// probe result is allowed to overwrite the trimmed ports again.
	m = send(m, keyMsg("p"))
	if m.items[0].edited {
		t.Fatal("re-probe did not clear edited")
	}
	waitUntil(t, func() bool { return !pm.anyRunning() })
	m.reconcile()
	if got := m.items[0].svc.Ports; len(got) != 2 {
		t.Fatalf("after re-probe ports = %v, want the discovered [3000 4206]", got)
	}
}

func Test_Picker_ProbeKeyStartsSelectedOnly(t *testing.T) {
	release := make(chan struct{})
	pm := newProbeManager(func(config.Service) ([]config.Port, error) {
		<-release // block so we can observe the running state deterministically
		return []config.Port{config.LiteralPort(9000)}, nil
	})
	m := buildPicker("t", []pickItem{
		{svc: config.Service{Name: "api"}, enabled: true},
		{svc: config.Service{Name: "off"}, enabled: false},
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
		{svc: config.Service{Name: "api", Runtime: "go", Go: &config.GoConfig{Package: ".", Args: []string{"serve"}}}, enabled: true},
		{svc: config.Service{Name: "db", Runtime: "docker"}, enabled: false},
	}, 0, false, nil)
	m.width = 100
	out := m.View()
	for _, want := range []string{"enabled 1/2 services", "SERVICE", "RUNTIME", "COMMAND", "PORTS", "go run . serve", "(disabled)"} {
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

func Test_Picker_AddPathGatedByAllow(t *testing.T) {
	// allowAddPath off: `f` is inert.
	off := send(newPicker("a"), keyMsg("f"))
	if off.result != resCancel {
		t.Fatalf("f with allowAddPath=false set result to %d, want unchanged", off.result)
	}
	m := newPicker("a")
	m.allowAddPath = true
	if got := send(m, keyMsg("f")).result; got != resAddPath {
		t.Fatalf("f result = %d, want resAddPath", got)
	}
}

func Test_RowCommand_ShowsExternalDir(t *testing.T) {
	tests := []struct {
		name string
		svc  config.Service
		want string
	}{
		{
			name: "root service shows the bare command",
			svc:  config.Service{Runtime: "go", Go: &config.GoConfig{Package: "./cmd/api"}},
			want: "go run ./cmd/api",
		},
		{
			name: "external service is prefixed with its dir",
			svc:  config.Service{Dir: "../ui", Runtime: "go", Go: &config.GoConfig{Package: "./cmd/web"}},
			want: "../ui · go run ./cmd/web",
		},
		{
			name: "dot dir is treated as root",
			svc:  config.Service{Dir: ".", Runtime: "shell", Commands: config.Commands{Run: &config.Command{Command: "make dev"}}},
			want: "make dev",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rowCommand(tt.svc); got != tt.want {
				t.Fatalf("rowCommand() = %q, want %q", got, tt.want)
			}
		})
	}
}
