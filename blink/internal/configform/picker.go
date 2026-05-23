package configform

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/toaweme/log"

	"github.com/toaweme/blink/core/config"
)

// ErrCanceled is returned by PickServices when the user quits the picker
// without confirming (q / esc / ctrl+c). Callers treat it as "write nothing".
var ErrCanceled = errors.New("canceled")

// PickServices runs the service picker: one screen listing every service with a
// select checkbox, where → drills into a per-service editor and enter saves. It
// returns the selected (and possibly edited or probed) services.
//
// detectFn, when non-nil, enables the re-detect key (`d`) and is called to fetch
// fresh services to merge by name. probeFn, when non-nil, enables the port-
// discovery key (`p`): it probes every selected service concurrently, animating
// a spinner per row, and fills in the ports each bound. Probes outlive a trip
// into the editor (owned by a manager that spans picker re-runs), so going in
// and back doesn't restart them.
func PickServices(title string, services []config.Service, detectFn func() ([]config.Service, error), probeFn func(config.Service) ([]config.Port, error)) ([]config.Service, error) {
	items := make([]pickItem, 0, len(services))
	for _, s := range services {
		items = append(items, pickItem{svc: s, keep: true})
	}
	cursor := 0

	var probes *probeManager
	if probeFn != nil {
		probes = newProbeManager(probeFn)
	}

	for {
		p := buildPicker(title, items, cursor, detectFn != nil, probes)
		out, err := tea.NewProgram(p, tea.WithAltScreen()).Run()
		if err != nil {
			return nil, fmt.Errorf("failed to run service picker: %w", err)
		}
		fp, ok := out.(picker)
		if !ok {
			return nil, fmt.Errorf("unexpected picker model type %T", out)
		}
		items = fp.items
		cursor = clamp(fp.cursor, 0, len(items)-1)

		switch fp.result {
		case resCancel:
			return nil, ErrCanceled

		case resDone:
			kept := make([]config.Service, 0, len(items))
			for _, it := range items {
				if it.keep {
					kept = append(kept, it.svc)
				}
			}
			return kept, nil

		case resEdit:
			if fp.editIdx < 0 || fp.editIdx >= len(items) {
				continue
			}
			if err := EditService(&items[fp.editIdx].svc, otherNames(items, fp.editIdx)); err != nil {
				return nil, err
			}

		case resAdd:
			ns := config.Service{Name: uniqueName("service", nameSet(items, -1)), Runtime: "shell"}
			if err := EditService(&ns, otherNames(items, -1)); err != nil {
				return nil, err
			}
			items = append(items, pickItem{svc: ns, keep: true})
			cursor = len(items) - 1

		case resDetect:
			fresh, derr := detectFn()
			if derr != nil {
				return nil, derr
			}
			have := nameSet(items, -1)
			for _, s := range fresh {
				if !have[s.Name] {
					items = append(items, pickItem{svc: s, keep: true})
					have[s.Name] = true
				}
			}
		}
	}
}

// probeState is the per-row runtime-discovery state.
type probeState int

const (
	probeIdle probeState = iota
	probeRunning
	probeDone
	probeNoPort
	probeFailed
)

type pickItem struct {
	svc   config.Service
	keep  bool
	state probeState
}

type pickResult int

const (
	resCancel pickResult = iota
	resDone
	resEdit
	resAdd
	resDetect
)

// probeOutcome is a finished probe's result, stored by service name.
type probeOutcome struct {
	ports []config.Port
	err   error
}

// probeManager owns the background probe goroutines and their results. It lives
// across picker re-runs (a trip into the editor quits and rebuilds the picker),
// so an in-flight probe keeps running and its result is still here when the
// picker comes back. It hushes logs while any probe runs.
type probeManager struct {
	fn func(config.Service) ([]config.Port, error)

	mu      sync.Mutex
	running map[string]bool
	results map[string]probeOutcome
	active  int
	hushed  bool
}

func newProbeManager(fn func(config.Service) ([]config.Port, error)) *probeManager {
	return &probeManager{fn: fn, running: map[string]bool{}, results: map[string]probeOutcome{}}
}

// start launches a probe for each service not already running, re-running
// already-probed services for a fresh reading.
func (pm *probeManager) start(svcs []config.Service) {
	pm.mu.Lock()
	var toRun []config.Service
	for _, s := range svcs {
		if pm.running[s.Name] {
			continue
		}
		pm.running[s.Name] = true
		pm.active++
		toRun = append(toRun, s)
	}
	if pm.active > 0 && !pm.hushed {
		hushLogs()
		pm.hushed = true
	}
	pm.mu.Unlock()

	for _, s := range toRun {
		go pm.run(s)
	}
}

func (pm *probeManager) run(svc config.Service) {
	ports, err := pm.fn(svc)
	pm.mu.Lock()
	delete(pm.running, svc.Name)
	pm.results[svc.Name] = probeOutcome{ports: ports, err: err}
	pm.active--
	if pm.active == 0 && pm.hushed {
		restoreLogs()
		pm.hushed = false
	}
	pm.mu.Unlock()
}

func (pm *probeManager) anyRunning() bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	return pm.active > 0
}

// snapshot copies the current running set and results for the renderer.
func (pm *probeManager) snapshot() (map[string]bool, map[string]probeOutcome) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	running := make(map[string]bool, len(pm.running))
	for k, v := range pm.running {
		if v {
			running[k] = true
		}
	}
	results := make(map[string]probeOutcome, len(pm.results))
	for k, v := range pm.results {
		results[k] = v
	}
	return running, results
}

// picker is the bubbletea model for one pass over the service list. Probing runs
// in the background via the manager and is reflected on each spinner tick;
// edit/add/detect quit and are handled by the outer PickServices loop, which
// re-runs a fresh picker.
type picker struct {
	title       string
	items       []pickItem
	cursor      int
	result      pickResult
	editIdx     int
	width       int
	allowDetect bool
	probes      *probeManager
	spinner     spinner.Model
	ticking     bool
}

var _ tea.Model = picker{}

func buildPicker(title string, items []pickItem, cursor int, allowDetect bool, probes *probeManager) picker {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	m := picker{title: title, items: items, cursor: cursor, allowDetect: allowDetect, probes: probes, spinner: sp}
	m.reconcile()
	return m
}

func (m picker) Init() tea.Cmd {
	if m.probes != nil && m.probes.anyRunning() {
		return m.spinner.Tick
	}
	return nil
}

func (m picker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		// repaint over a cleared buffer: when the terminal shrinks, the old frame
		// leaves artifacts the diff renderer won't wipe. This doesn't disturb an
		// in-flight spinner tick (that chain runs on its own TickMsg cmds).
		return m, tea.ClearScreen

	case spinner.TickMsg:
		m.reconcile()
		if m.probes == nil || !m.probes.anyRunning() {
			m.ticking = false
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		m.ticking = true
		return m, cmd

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m picker) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}
	case " ":
		if m.cursor < len(m.items) {
			m.items[m.cursor].keep = !m.items[m.cursor].keep
		}
	case "right", "l":
		if len(m.items) > 0 {
			m.result = resEdit
			m.editIdx = m.cursor
			return m, tea.Quit
		}
	case "left", "h":
		// reserved for "back"; the per-service editor handles its own back/cancel,
		// so at the list level there's nothing to collapse: a deliberate no-op.
	case "p":
		if m.probes != nil {
			m.probes.start(m.selectedServices())
			m.reconcile()
			if !m.ticking {
				m.ticking = true
				return m, m.spinner.Tick
			}
		}
	case "a":
		m.result = resAdd
		return m, tea.Quit
	case "d":
		if m.allowDetect {
			m.result = resDetect
			return m, tea.Quit
		}
	case "enter", "ctrl+s":
		m.result = resDone
		return m, tea.Quit
	case "esc", "ctrl+c":
		m.result = resCancel
		return m, tea.Quit
	}
	return m, nil
}

// reconcile folds the manager's current running set and results into the row
// states (and ports), so the view reflects probes that progressed while this
// picker was rebuilt or between ticks.
func (m picker) reconcile() {
	if m.probes == nil {
		return
	}
	running, results := m.probes.snapshot()
	for i := range m.items {
		name := m.items[i].svc.Name
		if running[name] {
			m.items[i].state = probeRunning
			continue
		}
		out, ok := results[name]
		if !ok {
			continue
		}
		switch {
		case out.err != nil:
			m.items[i].state = probeFailed
		case len(out.ports) == 0:
			m.items[i].state = probeNoPort
		default:
			m.items[i].svc.Ports = out.ports
			m.items[i].state = probeDone
		}
	}
}

func (m picker) selectedServices() []config.Service {
	var out []config.Service
	for _, it := range m.items {
		if it.keep {
			out = append(out, it.svc)
		}
	}
	return out
}

const (
	colGap     = 2
	rtWidth    = 8
	cyan       = lipgloss.Color("44")
	green      = lipgloss.Color("78")
	dimColor   = lipgloss.Color("244")
	faintColor = lipgloss.Color("240")
	portColor  = lipgloss.Color("75")
	envColor   = lipgloss.Color("180")
)

func (m picker) View() string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Foreground(cyan).Bold(true)
	dim := lipgloss.NewStyle().Foreground(dimColor)
	selected := 0
	for _, it := range m.items {
		if it.keep {
			selected++
		}
	}
	b.WriteString(titleStyle.Render(m.title))
	b.WriteString(dim.Render(fmt.Sprintf("   selected %d/%d services", selected, len(m.items))))
	b.WriteString("\n\n")

	if len(m.items) == 0 {
		b.WriteString(dim.Render("  no services - press a to add one\n"))
		b.WriteString("\n" + m.renderHints())
		return b.String()
	}

	nameW := m.nameWidth()
	cmdW := m.commandWidth(nameW)

	header := lipgloss.NewStyle().Foreground(faintColor).Bold(true)
	b.WriteString("  " + // arrow gutter (2)
		" " + // checkbox gutter (1 glyph)
		" " + // gap before name
		header.Render(padRight("SERVICE", nameW)) + strings.Repeat(" ", colGap) +
		header.Render(padRight("RUNTIME", rtWidth)) + strings.Repeat(" ", colGap) +
		header.Render(padRight("COMMAND", cmdW)) + strings.Repeat(" ", colGap) +
		header.Render("PORTS"))
	b.WriteString("\n")

	for i, it := range m.items {
		b.WriteString(m.renderRow(i, it, nameW, cmdW) + "\n")
	}

	b.WriteString("\n" + m.renderHints())
	return b.String()
}

func (m picker) renderRow(i int, it pickItem, nameW, cmdW int) string {
	dim := lipgloss.NewStyle().Foreground(dimColor)

	arrow := "  "
	if i == m.cursor {
		arrow = lipgloss.NewStyle().Foreground(cyan).Bold(true).Render("❯ ")
	}

	box := dim.Render("○")
	if it.keep {
		box = lipgloss.NewStyle().Foreground(green).Render("◉")
	}

	nameStyle := lipgloss.NewStyle().Bold(true)
	if !it.keep {
		nameStyle = dim.Strikethrough(true)
	}
	name := nameStyle.Render(padRight(it.svc.Name, nameW))

	rt := dim.Render(padRight(runtimeLabel(it.svc.Runtime), rtWidth))

	cmd := lipgloss.NewStyle().Foreground(faintColor).Render(padRight(truncate(serviceCommand(it.svc), cmdW), cmdW))

	return fmt.Sprintf("%s%s %s%s%s%s%s%s%s",
		arrow, box,
		name, strings.Repeat(" ", colGap),
		rt, strings.Repeat(" ", colGap),
		cmd, strings.Repeat(" ", colGap),
		m.renderPorts(it))
}

// renderPorts shows the spinner while a row is probing, the discovered ports
// when known, and a dim dash otherwise.
func (m picker) renderPorts(it pickItem) string {
	dim := lipgloss.NewStyle().Foreground(dimColor)
	switch it.state {
	case probeRunning:
		return m.spinner.View() + dim.Render(" scanning")
	case probeNoPort:
		return dim.Render("no port")
	case probeFailed:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("131")).Render("n/a")
	default:
		// probeIdle / probeDone: render the (possibly probe-filled) ports below.
	}
	if len(it.svc.Ports) == 0 {
		return dim.Render("—")
	}
	return renderPortList(it.svc.Ports)
}

func (m picker) renderHints() string {
	key := lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Bold(true)
	dim := lipgloss.NewStyle().Foreground(dimColor)
	sep := dim.Render("   ")
	hints := []string{
		key.Render("↑↓") + dim.Render(" move"),
		key.Render("space") + dim.Render(" select"),
		key.Render("→") + dim.Render(" edit"),
		key.Render("a") + dim.Render(" add"),
	}
	if m.allowDetect {
		hints = append(hints, key.Render("d")+dim.Render(" re-detect"))
	}
	if m.probes != nil {
		hints = append(hints, key.Render("p")+dim.Render(" probe ports"))
	}
	hints = append(hints,
		key.Render("enter")+dim.Render(" save"),
		key.Render("esc")+dim.Render(" cancel"),
	)
	return strings.Join(hints, sep)
}

func (m picker) nameWidth() int {
	w := len("SERVICE")
	for _, it := range m.items {
		if l := utf8.RuneCountInString(it.svc.Name); l > w {
			w = l
		}
	}
	if w > 24 {
		w = 24
	}
	return w
}

// commandWidth hugs the widest command (capped) so the table stays tight, then
// shrinks to the terminal's remaining budget so a row never wraps.
func (m picker) commandWidth(nameW int) int {
	w := len("COMMAND")
	for _, it := range m.items {
		if l := utf8.RuneCountInString(serviceCommand(it.svc)); l > w {
			w = l
		}
	}
	if w > 50 {
		w = 50
	}
	if m.width > 0 {
		const portsBudget = 12
		budget := m.width - (2 + 1 + 1 + nameW + colGap + rtWidth + colGap + colGap + portsBudget)
		if budget >= 8 && w > budget {
			w = budget
		}
	}
	return w
}

// runtimeLabel normalizes the empty runtime to its effective default.
func runtimeLabel(rt string) string {
	if rt == "" {
		return "shell"
	}
	return rt
}

// serviceCommand is the one-line invocation shown per row, including args so two
// services that share a package are still distinguishable.
func serviceCommand(svc config.Service) string {
	switch svc.Runtime {
	case "go":
		if svc.Go == nil || svc.Go.Package == "" {
			return "go run (package unset)"
		}
		cmd := "go run " + svc.Go.Package
		if len(svc.Go.Args) > 0 {
			cmd += " " + strings.Join(svc.Go.Args, " ")
		}
		return cmd
	case "docker":
		file := config.DefaultComposeFile
		if svc.Docker != nil && svc.Docker.File != "" {
			file = svc.Docker.File
		}
		return "compose up " + file
	case "node":
		pm, script := "npm", "dev"
		if svc.Node != nil {
			if svc.Node.PackageManager != "" {
				pm = svc.Node.PackageManager
			}
			if svc.Node.Script != "" {
				script = svc.Node.Script
			}
		}
		return pm + " run " + script
	default:
		if svc.Commands.Run != nil && svc.Commands.Run.Command != "" {
			return svc.Commands.Run.Command
		}
		return "(run command unset)"
	}
}

// renderPortList styles a service's ports: literals as ":8080" (port color),
// env references as the bare var name (env color, no colon since the value
// isn't known here).
func renderPortList(ports []config.Port) string {
	lit := lipgloss.NewStyle().Foreground(portColor)
	env := lipgloss.NewStyle().Foreground(envColor)
	parts := make([]string, 0, len(ports))
	for _, p := range ports {
		if p.EnvKey != "" {
			parts = append(parts, env.Render(p.EnvKey))
			continue
		}
		parts = append(parts, lit.Render(":"+strconv.Itoa(p.Value)))
	}
	return strings.Join(parts, " ")
}

func otherNames(items []pickItem, except int) []string {
	out := make([]string, 0, len(items))
	for i, it := range items {
		if i == except {
			continue
		}
		out = append(out, it.svc.Name)
	}
	return out
}

func nameSet(items []pickItem, except int) map[string]bool {
	set := make(map[string]bool, len(items))
	for i, it := range items {
		if i == except {
			continue
		}
		set[it.svc.Name] = true
	}
	return set
}

// uniqueName returns base if free, else base-2, base-3, ... so an added service
// never collides with an existing name.
func uniqueName(base string, taken map[string]bool) string {
	if !taken[base] {
		return base
	}
	for n := 2; ; n++ {
		c := base + "-" + strconv.Itoa(n)
		if !taken[c] {
			return c
		}
	}
}

// truncate clips s to maxLen runes, marking the cut with an ellipsis. A maxLen
// of 0 or less yields "" so a narrow terminal never wraps the row.
func truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= maxLen {
		return s
	}
	if maxLen == 1 {
		return "…"
	}
	return string(r[:maxLen-1]) + "…"
}

// padRight pads s with spaces to a display width of n columns, measuring in
// runes (not bytes) so multibyte content doesn't shift the columns after it.
func padRight(s string, n int) string {
	w := utf8.RuneCountInString(s)
	if w >= n {
		return s
	}
	return s + strings.Repeat(" ", n-w)
}

func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

var (
	logMu   sync.Mutex
	logPrev log.Logger
)

// hushLogs swaps the global logger for a discard one while a probe runs, so the
// throwaway supervisor's output doesn't bleed onto the picker's alt-screen.
func hushLogs() {
	logMu.Lock()
	defer logMu.Unlock()
	logPrev = log.Default()
	log.SetDefault(log.Discard())
}

func restoreLogs() {
	logMu.Lock()
	defer logMu.Unlock()
	if logPrev != nil {
		log.SetDefault(logPrev)
	}
}
