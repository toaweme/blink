package configform

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/toaweme/blink/core/config"
)

// ErrCanceled is returned by PickServices when the user quits the picker
// without confirming (q / esc / ctrl+c). Callers treat it as "write nothing".
var ErrCanceled = errors.New("canceled")

// PickServices runs the compact service picker: one screen listing every
// service with a keep/drop checkbox, where enter drills into a per-service
// editor instead of forcing every service through a wizard. It returns the
// kept (and possibly edited) services. detectFn, when non-nil, enables the
// re-detect key (`d`) and is called to fetch fresh services to merge by name.
func PickServices(title string, services []config.Service, detectFn func() ([]config.Service, error)) ([]config.Service, error) {
	items := make([]pickItem, len(services))
	for i, s := range services {
		items[i] = pickItem{svc: s, keep: true}
	}
	cursor := 0

	for {
		p := picker{title: title, items: items, cursor: cursor, allowDetect: detectFn != nil}
		out, err := tea.NewProgram(p, tea.WithAltScreen()).Run()
		if err != nil {
			return nil, fmt.Errorf("failed to run service picker: %w", err)
		}
		fp := out.(picker)
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

type pickItem struct {
	svc  config.Service
	keep bool
}

type pickResult int

const (
	resCancel pickResult = iota
	resDone
	resEdit
	resAdd
	resDetect
)

// picker is the bubbletea model for one pass over the service list. It quits as
// soon as the user picks an action (edit/add/detect/done/cancel); the outer
// PickServices loop performs the action and re-runs a fresh picker with the
// updated state, so only one bubbletea program is ever live at a time (the
// per-service editor is its own huh program).
type picker struct {
	title       string
	items       []pickItem
	cursor      int
	result      pickResult
	editIdx     int
	width       int
	allowDetect bool
}

var _ tea.Model = picker{}

func (m picker) Init() tea.Cmd { return nil }

func (m picker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil
	case tea.KeyMsg:
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
		case "enter":
			if len(m.items) > 0 {
				m.result = resEdit
				m.editIdx = m.cursor
				return m, tea.Quit
			}
		case "a":
			m.result = resAdd
			return m, tea.Quit
		case "d":
			if m.allowDetect {
				m.result = resDetect
				return m, tea.Quit
			}
		case "w", "ctrl+s":
			m.result = resDone
			return m, tea.Quit
		case "q", "esc", "ctrl+c":
			m.result = resCancel
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m picker) View() string {
	var b strings.Builder
	title := lipgloss.NewStyle().Foreground(lipgloss.Color("36")).Bold(true).Render(m.title)
	kept := 0
	for _, it := range m.items {
		if it.keep {
			kept++
		}
	}
	b.WriteString(title + lipgloss.NewStyle().Foreground(lipgloss.Color("244")).
		Render(fmt.Sprintf("  ·  %d of %d kept", kept, len(m.items))))
	b.WriteString("\n\n")

	if len(m.items) == 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("244")).
			Render("  no services - press a to add one\n"))
	}
	nameW := m.nameWidth()
	for i, it := range m.items {
		b.WriteString(m.renderRow(i, it, nameW) + "\n")
	}

	b.WriteString("\n" + m.renderHints())
	return b.String()
}

func (m picker) renderRow(i int, it pickItem, nameW int) string {
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	cursor := "  "
	if i == m.cursor {
		cursor = lipgloss.NewStyle().Foreground(lipgloss.Color("36")).Bold(true).Render("❯ ")
	}

	box := dim.Render("[ ]")
	if it.keep {
		box = lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Render("[x]")
	}

	nameStyle := lipgloss.NewStyle().Bold(true)
	if !it.keep {
		nameStyle = dim.Strikethrough(true)
	}
	name := nameStyle.Render(padRight(it.svc.Name, nameW))

	rt := dim.Render(padRight(runtimeLabel(it.svc.Runtime), 7))

	portStr := portList(it.svc.Ports)
	// keep the row on one line: budget the summary against the terminal width
	// after the fixed-width columns and the ports, so a long run command
	// truncates with an ellipsis instead of wrapping.
	summary := serviceSummary(it.svc)
	if m.width > 0 {
		used := 2 + 4 + nameW + 1 + 8 + 2 // cursor, box, name, gap, runtime, gaps
		if portStr != "" {
			used += len(portStr) + 2
		}
		summary = truncate(summary, m.width-used)
	}

	ports := ""
	if portStr != "" {
		ports = "  " + lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Render(portStr)
	}

	return fmt.Sprintf("%s%s %s %s %s%s", cursor, box, name, rt, dim.Render(summary), ports)
}

func (m picker) renderHints() string {
	key := lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Bold(true)
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	sep := dim.Render("  ·  ")
	hints := []string{
		key.Render("space") + dim.Render(" keep/drop"),
		key.Render("enter") + dim.Render(" edit"),
		key.Render("a") + dim.Render(" add"),
	}
	if m.allowDetect {
		hints = append(hints, key.Render("d")+dim.Render(" re-detect"))
	}
	hints = append(hints,
		key.Render("w")+dim.Render(" write"),
		key.Render("q")+dim.Render(" cancel"),
	)
	return strings.Join(hints, sep)
}

func (m picker) nameWidth() int {
	w := 4
	for _, it := range m.items {
		if l := len(it.svc.Name); l > w {
			w = l
		}
	}
	if w > 24 {
		w = 24
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

// serviceSummary is the one-line "what does this run" shown per row.
func serviceSummary(svc config.Service) string {
	switch svc.Runtime {
	case "go":
		if svc.Go != nil && svc.Go.Package != "" {
			return svc.Go.Package
		}
		return "(go package unset)"
	case "docker":
		if svc.Docker != nil && svc.Docker.File != "" {
			return svc.Docker.File
		}
		return config.DefaultComposeFile
	default:
		if svc.Commands.Run != nil && svc.Commands.Run.Command != "" {
			return svc.Commands.Run.Command
		}
		return "(run command unset)"
	}
}

func portList(ports []int) string {
	parts := make([]string, 0, len(ports))
	for _, p := range ports {
		parts = append(parts, ":"+strconv.Itoa(p))
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

// truncate clips s to max runes, marking the cut with an ellipsis. A max of 0
// or less yields "" so a narrow terminal never wraps the row.
func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max == 1 {
		return "…"
	}
	return string(r[:max-1]) + "…"
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
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
