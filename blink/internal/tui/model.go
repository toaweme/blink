package tui

import (
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/toaweme/blink/core/control"
)

const (
	allTab           = "all"
	maxBufferedLines = 5000
	pulseInterval    = 700 * time.Millisecond
)

// accentColor is the shared teal/emerald used for the active/current/selected
// thing across the UI (active tab, focused-container badge).
const accentColor = "36"

// Controller is the session-action sink the TUI dispatches into. Only session
// actions reach here; view actions stay in the model.
type Controller interface {
	Dispatch(action control.Action, service string) error
}

// tickMsg drives the soft animations (pulsing running dot, idle countdown).
type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(pulseInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// Model is the bubbletea model for the blink TUI.
type Model struct {
	services []string
	statuses map[string]string
	buffers  map[string][]string

	// childList[service] is the ordered, deduped set of container names seen for
	// a runtime-managed service (docker compose). Drives the in-tab container ring.
	childList map[string][]string
	// childFocus[service] is the container the service tab is filtered to. Empty
	// or absent means the merged "all containers" view. Focused-container buffers
	// live under the composite key service+childSep+name.
	childFocus map[string]string

	tabs   []string
	active int

	// tabHistory is the visited-tab trail (browser back/forward). Every recorded
	// move truncates any forward entries and appends; histBack/histForward replay
	// it without recording. histPos is the current index into tabHistory.
	tabHistory []int
	histPos    int

	vp      viewport.Model
	width   int
	height  int
	keymap  control.Keymap
	control Controller
	ready   bool

	followTail bool

	// per-tab scroll state so switching tabs returns to where the user was.
	scrollState map[string]tabScroll

	// modalScroll is the line offset for the command-center modal, used when its
	// body is taller than the terminal. Reset to 0 on open; j/k or up/down adjusts.
	modalScroll int

	// helpOpen is true while the command-center/help modal is visible.
	helpOpen bool

	// rawMode (toggled with z) tears down the TUI overlay: bubbletea exits
	// alt-screen, mouse capture is disabled, View() returns empty, and new lines
	// stream via tea.Println into the native scrollback, where native scroll and
	// mouse-select work.
	rawMode bool

	// animation
	spinner    spinner.Model
	pulsePhase int

	// wrappedToBuffer[i] is the buffer index that wrapped visual row i was emitted
	// from. Populated by renderBufferMapped; used to keep the cursor row visible
	// while scrolling.
	wrappedToBuffer []int

	// startedAt[svc] is when the service most recently transitioned into "running".
	// Cleared on any non-running status. Drives the footer uptime readout.
	startedAt map[string]time.Time

	// reloads[svc] counts restarts since the TUI started, incremented on each
	// "restarting"/"building" to "running" cycle.
	reloads map[string]int

	// ports[svc] is the set of local TCP ports a service listens on. Seeded from
	// the probed/configured ports and, for runtime-managed services (docker),
	// updated from the published ports a StatusMsg carries once the stack is up.
	// Rendered as a "http://127.0.0.1:<port>, ..." address left of the uptime.
	ports map[string][]int

	// watchFiles, watchDirs and watchPerSvc are the latest counts published by
	// the supervisor via WatchStatsMsg. Zero before the first message.
	watchFiles  int
	watchDirs   int
	watchPerSvc map[string]WatchStat

	// cursorMode gates the line cursor + selection. Off by default (↑/↓ scroll the
	// viewport). Toggled with e; on, ↑/↓ move the cursor and the selection keys
	// (space, shift+↑/↓) become live.
	cursorMode bool
	// tabCursor[tab] is the buffer index the cursor is parked on for that tab.
	// Defaults to the tail line for new tabs (see ensureCursor).
	tabCursor map[string]int
	// selected[tab] is the set of buffer indices selected on that tab. The set
	// allows gaps. Selection is stateless: no anchor, only the rows in the set.
	// Cleared when cursor mode exits.
	selected map[string]map[int]bool

	// log sink controls, injected via WithLogControl. logDir is where selection
	// writes land (<svc>.selected.log); logsOn mirrors the sink state for the
	// footer; logToggle flips the sink live (nil = no-op).
	logDir    string
	logsOn    bool
	logToggle func() bool

	// flash is a transient badge (e.g. COPIED, WRITTEN) shown in the top-right for
	// flashDuration after an action. The pulse tick re-renders, so it fades on its
	// own without a timer.
	flash      string
	flashColor string
	flashAt    time.Time
}

// flashDuration is how long a transient action badge (COPIED/WRITTEN) stays
// visible before the next render drops it.
const flashDuration = 1500 * time.Millisecond

// tabScroll captures where the viewport was sitting for a given tab so it can
// be restored on return. When followTail is set, yOffset is ignored and restore
// snaps to the bottom.
type tabScroll struct {
	yOffset    int
	followTail bool
}

// NewModel builds a TUI model for the given services, wiring the controller
// that session actions dispatch into.
func NewModel(services []string, ctrl Controller) *Model {
	tabs := append([]string{allTab}, services...)
	statuses := make(map[string]string, len(services))
	buffers := make(map[string][]string, len(services)+1)
	buffers[allTab] = nil
	for _, s := range services {
		statuses[s] = "pending"
		buffers[s] = nil
	}

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))

	return &Model{
		services:    services,
		statuses:    statuses,
		buffers:     buffers,
		tabs:        tabs,
		active:      0,
		keymap:      control.DefaultKeymap(),
		control:     ctrl,
		followTail:  true,
		scrollState: make(map[string]tabScroll, len(services)+1),
		spinner:     sp,
		tabCursor:   map[string]int{},
		selected:    map[string]map[int]bool{},
		childList:   map[string][]string{},
		childFocus:  map[string]string{},
		tabHistory:  []int{0},
	}
}

// gotoTab switches to tab idx and records the move in the back/forward trail.
// No-op when already there, so a repeated jump doesn't pile up history.
func (m *Model) gotoTab(idx int) {
	if idx == m.active {
		return
	}
	m.saveScroll()
	m.active = idx
	// drop any forward trail, then append and advance to the new tip.
	m.tabHistory = append(m.tabHistory[:m.histPos+1:m.histPos+1], idx)
	m.histPos = len(m.tabHistory) - 1
	m.restoreScroll()
}

// histBack and histForward replay the visited-tab trail without recording, so
// it behaves like browser history: back then a fresh jump forks a new branch,
// back then forward returns to the prior tab.
func (m *Model) histBack() {
	if m.histPos <= 0 {
		return
	}
	m.saveScroll()
	m.histPos--
	m.active = m.tabHistory[m.histPos]
	m.restoreScroll()
}

func (m *Model) histForward() {
	if m.histPos >= len(m.tabHistory)-1 {
		return
	}
	m.saveScroll()
	m.histPos++
	m.active = m.tabHistory[m.histPos]
	m.restoreScroll()
}

// childSep joins a service and a focused container into one composite key. NUL
// never appears in a service or container name, so it can't collide with a
// plain tab key.
const childSep = "\x00"

// viewKey is the key the active view's buffer, scroll, cursor and selection
// state live under: the tab name, or a service+container composite while a
// container is focused. Tab-identity logic keeps using activeTab().
func (m *Model) viewKey() string {
	tab := m.activeTab()
	if c := m.childFocus[tab]; c != "" {
		return tab + childSep + c
	}
	return tab
}

// noteChild records a freshly seen container for a service in first-seen order.
// Idempotent.
func (m *Model) noteChild(service, child string) {
	if m.childList == nil {
		m.childList = map[string][]string{}
	}
	for _, c := range m.childList[service] {
		if c == child {
			return
		}
	}
	m.childList[service] = append(m.childList[service], child)
}

// appendChildLine stores a raw (unprefixed) container line in its own buffer so
// a focused container tab renders clean output. The prefixed copy still lands in
// the merged service buffer and the all-tab via appendLine.
func (m *Model) appendChildLine(service, child, line string) {
	key := service + childSep + child
	buf := append(m.buffers[key], line)
	if len(buf) > maxBufferedLines {
		buf = buf[len(buf)-maxBufferedLines:]
	}
	m.buffers[key] = buf
}

// cycleChild advances the active service tab's container focus around the ring:
// all, first container, ..., last, all. No-op on tabs with no containers.
func (m *Model) cycleChild(delta int) {
	tab := m.activeTab()
	children := m.childList[tab]
	if len(children) == 0 {
		return
	}
	m.saveScroll()
	// ring index 0 is the merged "all" view; 1..n map to children[idx-1].
	idx := 0
	if cur := m.childFocus[tab]; cur != "" {
		for i, c := range children {
			if c == cur {
				idx = i + 1
				break
			}
		}
	}
	n := len(children) + 1
	idx = (idx + delta + n) % n
	if m.childFocus == nil {
		m.childFocus = map[string]string{}
	}
	if idx == 0 {
		delete(m.childFocus, tab)
	} else {
		m.childFocus[tab] = children[idx-1]
	}
	m.restoreScroll()
}

// Init starts the spinner and the heartbeat tick that drive the soft animations.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, tickCmd())
}

// Update routes incoming messages to per-type handlers. Heavy logic lives in
// dedicated helpers (handleKey and per-modal subfunctions).
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.handleWindowSize(msg)
	case LineMsg:
		return m.handleLineMsg(msg)
	case StatusMsg:
		return m.handleStatusMsg(msg)
	case tickMsg:
		m.pulsePhase++
		return m, tickCmd()
	case refreshMsg:
		// fired after exitRawMode reattaches the alt-screen; restore the
		// pre-zen viewport offset.
		m.restoreScroll()
		return m, nil
	case WatchStatsMsg:
		m.watchFiles = msg.Files
		m.watchDirs = msg.Dirs
		m.watchPerSvc = msg.PerSvc
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case tea.MouseMsg:
		// horizontal wheel switches tabs (trackpad swipe, shift+wheel on most
		// mice). Press-only to avoid rapid-fire double events.
		if msg.Action == tea.MouseActionPress {
			switch msg.Button {
			case tea.MouseButtonWheelLeft:
				m.gotoTab((m.active - 1 + len(m.tabs)) % len(m.tabs))
				return m, nil
			case tea.MouseButtonWheelRight:
				m.gotoTab((m.active + 1) % len(m.tabs))
				return m, nil
			default:
				// other buttons fall through to vertical-wheel handling below.
			}
		}
		// in cursor mode the vertical wheel moves the cursor and the viewport
		// follows; otherwise it falls through to the viewport for plain scrolling.
		if m.cursorMode && msg.Action == tea.MouseActionPress {
			switch msg.Button {
			case tea.MouseButtonWheelUp:
				m.moveCursor(-scrollStep)
				m.refreshViewport()
				return m, nil
			case tea.MouseButtonWheelDown:
				m.moveCursor(scrollStep)
				m.refreshViewport()
				return m, nil
			default:
				// non-wheel buttons fall through to the viewport.
			}
		}
		// the vertical wheel falls through to the viewport for scrolling.
		// Left-click is deliberately unbound: mouse reporting is unreliable
		// across terminals, so the line cursor is keyboard-driven (e then ↑/↓).
	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	m.followTail = m.vp.AtBottom()
	return m, cmd
}

func (m *Model) handleWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height
	w, h := m.viewportSize()
	if !m.ready {
		m.vp = viewport.New(w, h)
		m.ready = true
	} else {
		m.vp.Width = w
		m.vp.Height = h
	}
	m.refreshViewport()
	return m, nil
}

func (m *Model) handleLineMsg(msg LineMsg) (tea.Model, tea.Cmd) {
	line := msg.Line
	if msg.Child != "" {
		// raw line into the per-container buffer (for a focused tab); the
		// prefixed copy still flows into the merged service and all views.
		m.noteChild(msg.Service, msg.Child)
		m.appendChildLine(msg.Service, msg.Child, msg.Line)
		line = lipgloss.NewStyle().Faint(true).Render("["+msg.Child+"]") + " " + line
	}
	m.appendLine(msg.Service, line)
	if m.rawMode {
		// stream only the view the user has tabbed to into the native terminal,
		// so zen mode can filter to one service or container while still handing
		// the screen back for scroll and select.
		if out, ok := m.rawTail(msg.Service, msg.Child, line, msg.Line); ok {
			return m, tea.Println(out)
		}
		return m, nil
	}
	if m.activeTab() == allTab || m.activeTab() == msg.Service {
		m.refreshViewportFollow()
	}
	return m, nil
}

func (m *Model) handleStatusMsg(msg StatusMsg) (tea.Model, tea.Cmd) {
	// ports are keyed like buffers: the bare service for the merged/service-level
	// view, the service+container composite for a focused container. A per-child
	// event carries that one container's ports so the focused view shows only it.
	if len(msg.Ports) > 0 {
		if m.ports == nil {
			m.ports = map[string][]int{}
		}
		key := msg.Service
		if msg.Child != "" {
			key = msg.Service + childSep + msg.Child
		}
		m.ports[key] = msg.Ports
	}
	if msg.Child == "" {
		prev := m.statuses[msg.Service]
		m.statuses[msg.Service] = msg.Status
		if msg.Status == "running" && prev != "running" {
			if m.startedAt == nil {
				m.startedAt = map[string]time.Time{}
			}
			m.startedAt[msg.Service] = time.Now()
			if prev != "" && prev != "pending" {
				if m.reloads == nil {
					m.reloads = map[string]int{}
				}
				m.reloads[msg.Service]++
			}
		} else if msg.Status != "running" {
			delete(m.startedAt, msg.Service)
		}
	}
	if msg.Err != nil {
		m.appendLine(msg.Service, lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Render("error: "+msg.Err.Error()))
	}
	label := "── " + msg.Status + " ──"
	childLabel := ""
	if msg.Child != "" {
		// surface the container so it joins the in-tab ring before its first log
		// line, and give its focused view the bare status marker.
		m.noteChild(msg.Service, msg.Child)
		childLabel = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render("── " + msg.Status + " ──")
		m.appendChildLine(msg.Service, msg.Child, childLabel)
		label = "── " + msg.Child + ": " + msg.Status + " ──"
	}
	labelLine := lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render(label)
	m.appendLine(msg.Service, labelLine)
	if m.rawMode {
		if out, ok := m.rawTail(msg.Service, msg.Child, labelLine, childLabel); ok {
			return m, tea.Println(out)
		}
		return m, nil
	}
	if m.activeTab() == allTab || m.activeTab() == msg.Service {
		m.refreshViewportFollow()
	}
	return m, nil
}

// handleKey is the top-level key dispatcher. Modal overlays (command-center,
// raw-mode) get first refusal; anything they don't consume falls through to
// the global keymap.
func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.helpOpen {
		return m.handleCommandCenterKey(msg)
	}
	if m.rawMode {
		return m.handleRawModeKey(msg)
	}
	return m.handleGlobalKey(msg)
}

// handleCommandCenterKey owns the / and ? command-center overlay: close and
// modal scroll.
func (m *Model) handleCommandCenterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if a, ok := m.keymap.Lookup(msg.String()); ok && a == control.ActionCommandCenter {
		m.helpOpen = false
		return m, nil
	}
	switch {
	case msg.String() == "esc", msg.String() == "q":
		m.helpOpen = false
		return m, nil
	case msg.String() == "ctrl+c":
		// q is swallowed while the help modal is open; ctrl+c still tears down.
		return m, tea.Quit
	}
	// scroll the modal when its body overflows the viewport.
	switch msg.String() {
	case "up", "k":
		m.modalScroll--
		return m, nil
	case "down", "j":
		m.modalScroll++
		return m, nil
	case "pgup":
		m.modalScroll -= 10
		return m, nil
	case "pgdown":
		m.modalScroll += 10
		return m, nil
	case "home":
		m.modalScroll = 0
		return m, nil
	case "end":
		m.modalScroll = 1 << 30
		return m, nil
	}
	return m, nil
}

// handleRawModeKey is the minimal keymap active while the TUI has handed the
// screen back to the user. Quit and z (exit zen) respond, plus tab and container
// navigation so zen mode can filter which service/container streams; everything
// else passes through to the native terminal.
func (m *Model) handleRawModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch a, _ := m.keymap.Lookup(msg.String()); a {
	case control.ActionQuit:
		return m, tea.Quit
	case control.ActionToggleZen:
		return m, m.exitRawMode()
	case control.ActionNextTab:
		return m, m.rawNav(func() { m.gotoTab((m.active + 1) % len(m.tabs)) })
	case control.ActionPrevTab:
		return m, m.rawNav(func() { m.gotoTab((m.active - 1 + len(m.tabs)) % len(m.tabs)) })
	case control.ActionNextChild:
		return m, m.rawNav(func() { m.cycleChild(1) })
	case control.ActionPrevChild:
		return m, m.rawNav(func() { m.cycleChild(-1) })
	case control.ActionHistBack:
		return m, m.rawNav(m.histBack)
	case control.ActionHistForward:
		return m, m.rawNav(m.histForward)
	default:
		// every other action is swallowed in raw mode.
	}
	return m, nil
}

// rawTail returns the text to stream into native scrollback for a line belonging
// to service/child, or ok=false when that line is outside the view the user has
// tabbed to in zen mode. The rendering mirrors what the focused view's buffer
// holds so a tab switch (rawFlush) and live streaming look identical: the all tab
// gets the tinted, prefixed form; a service tab its merged line; a focused
// container its bare line. merged is the buffer line for the service/all views,
// raw the unprefixed container line.
func (m *Model) rawTail(service, child, merged, raw string) (string, bool) {
	tab := m.activeTab()
	switch {
	case tab == allTab:
		prefix := serviceStyle(service).Render("["+service+"]") + " "
		return serviceTintStyle(service).Render(prefix + merged), true
	case tab != service:
		return "", false
	default:
		if c := m.childFocus[tab]; c != "" {
			if child != c {
				return "", false
			}
			return raw, true
		}
		return merged, true
	}
}

// rawNav applies a zen-mode focus change and, when the focused view actually
// moved, flushes the newly focused view's backlog into native scrollback so the
// user lands on content instead of waiting for the next line.
func (m *Model) rawNav(nav func()) tea.Cmd {
	before := m.viewKey()
	nav()
	if m.viewKey() == before {
		return nil
	}
	return m.rawFlush()
}

// rawFlush pushes the focused view's buffered backlog into native scrollback,
// headed by a marker naming the view, after a zen-mode tab or container switch.
func (m *Model) rawFlush() tea.Cmd {
	header := lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render("── " + m.rawFocusLabel() + " ──")
	if buf := m.buffers[m.viewKey()]; len(buf) > 0 {
		return tea.Println(header + "\n" + strings.Join(buf, "\n"))
	}
	return tea.Println(header)
}

// rawFocusLabel names the view zen mode is currently streaming: the tab, or
// "tab · container" while a container is focused.
func (m *Model) rawFocusLabel() string {
	tab := m.activeTab()
	if c := m.childFocus[tab]; c != "" {
		return tab + " · " + c
	}
	return tab
}

// scrollStep is how many lines ↑/↓ move the viewport in scroll mode.
const scrollStep = 3

// handleGlobalKey is the main keymap: tab navigation, restart, action verbs.
// Reached when no modal owns the input.
func (m *Model) handleGlobalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// fixed scroll navigation, outside the rebindable keymap, so log navigation
	// is always fast. In cursor mode these drive the cursor and the viewport
	// follows it; in scroll mode they move the viewport directly.
	switch msg.String() {
	case "pgup":
		if m.cursorMode {
			m.moveCursor(-m.vp.Height)
			m.refreshViewport()
			return m, nil
		}
		m.vp.PageUp()
		m.followTail = m.vp.AtBottom()
		return m, nil
	case "pgdown":
		if m.cursorMode {
			m.moveCursor(m.vp.Height)
			m.refreshViewport()
			return m, nil
		}
		m.vp.PageDown()
		m.followTail = m.vp.AtBottom()
		return m, nil
	case "ctrl+u":
		if m.cursorMode {
			m.moveCursor(-m.vp.Height / 2)
			m.refreshViewport()
			return m, nil
		}
		m.vp.HalfPageUp()
		m.followTail = m.vp.AtBottom()
		return m, nil
	case "ctrl+d":
		if m.cursorMode {
			m.moveCursor(m.vp.Height / 2)
			m.refreshViewport()
			return m, nil
		}
		m.vp.HalfPageDown()
		m.followTail = m.vp.AtBottom()
		return m, nil
	case "home", "g":
		if m.cursorMode {
			m.jumpCursorTo(0)
			m.refreshViewport()
			return m, nil
		}
		m.vp.GotoTop()
		m.followTail = false
		return m, nil
	case "end", "G":
		if m.cursorMode {
			m.jumpCursorTo(len(m.buffers[m.viewKey()]) - 1)
			m.refreshViewport()
			return m, nil
		}
		m.vp.GotoBottom()
		m.followTail = true
		return m, nil
	}
	switch a, _ := m.keymap.Lookup(msg.String()); a {
	case control.ActionQuit:
		return m, tea.Quit
	case control.ActionCommandCenter:
		m.helpOpen = true
		m.modalScroll = 0
		return m, nil
	case control.ActionWriteSelection:
		m.writeSelection()
		m.refreshViewport()
		return m, nil
	case control.ActionAppendSelection:
		m.appendSelection()
		m.refreshViewport()
		return m, nil
	case control.ActionToggleLogs:
		if m.logToggle != nil {
			m.logsOn = m.logToggle()
		}
		return m, nil
	case control.ActionToggleZen:
		return m, m.enterRawMode()
	case control.ActionNextTab:
		m.gotoTab((m.active + 1) % len(m.tabs))
		return m, nil
	case control.ActionPrevTab:
		m.gotoTab((m.active - 1 + len(m.tabs)) % len(m.tabs))
		return m, nil
	case control.ActionHistBack:
		m.histBack()
		return m, nil
	case control.ActionHistForward:
		m.histForward()
		return m, nil
	case control.ActionNextChild:
		m.cycleChild(1)
		return m, nil
	case control.ActionPrevChild:
		m.cycleChild(-1)
		return m, nil
	case control.ActionRestart:
		if name := m.activeTab(); name != allTab && m.control != nil {
			_ = m.control.Dispatch(control.ActionRestart, name)
		}
		return m, nil
	case control.ActionInsertBlank:
		// inject a blank spacer into the focused service's output. It flows back
		// through the Hub into the buffer and the .log file; no local append here
		// or it would double. No-op on the all-tab.
		if name := m.activeTab(); name != allTab && m.control != nil {
			_ = m.control.Dispatch(control.ActionInsertBlank, name)
		}
		return m, nil
	case control.ActionRestartAll:
		if m.control != nil {
			_ = m.control.Dispatch(control.ActionRestartAll, "")
		}
		return m, nil
	case control.ActionClear:
		return m.handleClear()
	case control.ActionClearAll:
		return m.handleClearAll()
	case control.ActionCursorMode:
		m.toggleCursorMode()
		m.refreshViewport()
		return m, nil
	case control.ActionCursorUp:
		if m.cursorMode {
			m.moveCursor(-1)
			m.refreshViewport()
			return m, nil
		}
		m.vp.ScrollUp(scrollStep)
		m.followTail = m.vp.AtBottom()
		return m, nil
	case control.ActionCursorDown:
		if m.cursorMode {
			m.moveCursor(1)
			m.refreshViewport()
			return m, nil
		}
		m.vp.ScrollDown(scrollStep)
		m.followTail = m.vp.AtBottom()
		return m, nil
	case control.ActionExtendUp:
		m.extendSelection(-1)
		m.refreshViewport()
		return m, nil
	case control.ActionExtendDown:
		m.extendSelection(1)
		m.refreshViewport()
		return m, nil
	case control.ActionToggleSelect:
		m.toggleSelect()
		m.refreshViewport()
		return m, nil
	case control.ActionCopy:
		m.copySelection()
		m.refreshViewport()
		return m, nil
	case control.ActionClearCursor:
		m.escapeCursor()
		m.refreshViewport()
		return m, nil
	}
	// numeric jump 1..9
	if len(msg.Runes) == 1 {
		r := msg.Runes[0]
		if r >= '1' && r <= '9' {
			idx := int(r - '0')
			if idx < len(m.tabs) {
				m.gotoTab(idx)
			}
		}
	}
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	m.followTail = m.vp.AtBottom()
	return m, cmd
}

// handleClear empties the active tab's buffer. Terminal-only: never touches disk.
func (m *Model) handleClear() (tea.Model, tea.Cmd) {
	m.clearTab(m.viewKey())
	m.refreshViewport()
	return m, nil
}

// handleClearAll empties every buffer. Terminal-only: never touches disk.
func (m *Model) handleClearAll() (tea.Model, tea.Cmd) {
	for tab := range m.buffers {
		m.buffers[tab] = nil
	}
	m.refreshViewport()
	return m, nil
}

// enterRawMode tears down the bubbletea overlay so the user can use native
// scroll and mouse-select. Order matters: tea.Println is dropped while the
// alt-screen is active, so this sequences ExitAltScreen, DisableMouse,
// Println(buffer) instead of batching.
func (m *Model) enterRawMode() tea.Cmd {
	m.saveScroll()
	m.rawMode = true
	// release mouse capture so native scroll and select work.
	cmds := []tea.Cmd{tea.ExitAltScreen, tea.DisableMouse}
	// flush the current buffer into the native scrollback so the user lands on
	// real content instead of an empty screen.
	if buf := m.buffers[m.viewKey()]; len(buf) > 0 {
		cmds = append(cmds, tea.Println(strings.Join(buf, "\n")))
	}
	return tea.Sequence(cmds...)
}

// exitRawMode brings the overlay back and re-captures the mouse.
func (m *Model) exitRawMode() tea.Cmd {
	m.rawMode = false
	// re-capture the mouse (cell motion is on for the whole TUI lifetime).
	cmds := []tea.Cmd{tea.EnterAltScreen, tea.EnableMouseCellMotion}
	// schedule a refresh after re-entering the alt screen.
	cmds = append(cmds, func() tea.Msg { return refreshMsg{} })
	return tea.Batch(cmds...)
}

// refreshMsg is dispatched after re-entering alt-screen so the next Update tick
// re-renders the active tab.
type refreshMsg struct{}

// saveScroll snapshots where the active tab's viewport is parked.
func (m *Model) saveScroll() {
	if !m.ready {
		return
	}
	m.scrollState[m.viewKey()] = tabScroll{
		yOffset:    m.vp.YOffset,
		followTail: m.followTail,
	}
}

// restoreScroll repositions the viewport to wherever the now-active tab was
// last seen. Brand-new tabs default to tail-following.
func (m *Model) restoreScroll() {
	if !m.ready {
		return
	}
	// in cursor mode every tab carries a visible cursor, so seed one for the
	// now-active tab before rendering instead of waiting for the first ↑/↓.
	if m.cursorMode {
		m.ensureCursor()
	}
	// rerender the now-active tab before repositioning.
	content, mapping := m.renderBufferMapped(m.buffers[m.viewKey()])
	m.wrappedToBuffer = mapping
	m.vp.SetContent(content)
	s, ok := m.scrollState[m.viewKey()]
	if !ok {
		m.followTail = true
		m.vp.GotoBottom()
		return
	}
	m.followTail = s.followTail
	if s.followTail {
		m.vp.GotoBottom()
	} else {
		m.vp.SetYOffset(s.yOffset)
	}
}

// viewportSize returns the width and height available to the log viewport,
// accounting for the header, footer, and the one-line top padding View()
// prepends.
func (m *Model) viewportSize() (int, int) {
	h := m.height - lipgloss.Height(m.renderTabs()) - lipgloss.Height(m.renderFooter()) - topPaddingLines
	if h < 1 {
		h = 1
	}
	return m.width, h
}

// topPaddingLines is the number of blank lines View() emits before the header.
// A const so every height calculation that subtracts chrome stays consistent.
const topPaddingLines = 1

// View renders the current frame: tab header, log viewport, and footer. Returns
// empty in raw mode (the native terminal owns the screen).
func (m *Model) View() string {
	if !m.ready {
		return "starting blink..."
	}
	// raw mode: yield the screen so native scroll and mouse-select work. New
	// lines are pushed via tea.Println into the main screen buffer.
	if m.rawMode {
		return ""
	}
	if m.helpOpen {
		return m.renderHelpDialog()
	}
	footer := m.renderFooter()
	return strings.Repeat("\n", topPaddingLines) + m.renderTabs() + "\n" + m.vp.View() + "\n" + footer
}

func (m *Model) activeTab() string { return m.tabs[m.active] }

func (m *Model) clearTab(tab string) {
	m.buffers[tab] = nil
}

func (m *Model) appendLine(service, line string) {
	buf := append(m.buffers[service], line)
	if len(buf) > maxBufferedLines {
		buf = buf[len(buf)-maxBufferedLines:]
	}
	m.buffers[service] = buf

	prefix := serviceStyle(service).Render("["+service+"]") + " "
	// in the all-tab each line gets a per-service background tint so consecutive
	// lines group by service without re-parsing the prefix.
	tinted := serviceTintStyle(service).Render(prefix + line)
	all := append(m.buffers[allTab], tinted)
	if len(all) > maxBufferedLines {
		all = all[len(all)-maxBufferedLines:]
	}
	m.buffers[allTab] = all
}

// refreshViewport rerenders the active tab's content while preserving the scroll
// offset. Tail-following on new lines is opt-in via refreshViewportFollow;
// everything else goes through refreshViewport so it never moves the user.
func (m *Model) refreshViewport() {
	if !m.ready {
		return
	}
	offset := m.vp.YOffset
	content, mapping := m.renderBufferMapped(m.buffers[m.viewKey()])
	m.wrappedToBuffer = mapping
	m.vp.SetContent(content)
	m.vp.SetYOffset(offset)
}

// refreshViewportFollow is the new-line variant: rerenders and, if already
// parked at the tail, snaps to the new bottom.
func (m *Model) refreshViewportFollow() {
	if !m.ready {
		return
	}
	follow := m.followTail
	offset := m.vp.YOffset
	content, mapping := m.renderBufferMapped(m.buffers[m.viewKey()])
	m.wrappedToBuffer = mapping
	m.vp.SetContent(content)
	if follow {
		m.vp.GotoBottom()
	} else {
		m.vp.SetYOffset(offset)
	}
}

// renderBufferMapped renders the active buffer with the cursor gutter and
// selection tint, returning a parallel slice whose i-th entry is the buffer
// index that wrapped visual row i came from. The mapping enables mouse
// hit-testing.
func (m *Model) renderBufferMapped(lines []string) (string, []int) {
	if len(lines) == 0 {
		return "", nil
	}
	w := m.vp.Width
	if w <= 0 {
		w = 80
	}
	// the cursor and selection only render in cursor mode; scroll mode shows a
	// clean buffer with no gutter markers.
	headIdx := -1
	var selected map[int]bool
	if m.cursorMode {
		headIdx = m.cursorAt()
		selected = m.selected[m.viewKey()]
	}
	// all three gutter markers use the same left-edge glyph so the cursor and
	// selection bars line up exactly; only the color differs.
	marker := func(color string) string {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Bold(true).Render("▌")
	}
	cursorMarker := marker("220")
	selMarker := marker("82")
	bothMarker := marker("44")
	var b strings.Builder
	mapping := make([]int, 0, len(lines))
	for i, line := range lines {
		shown := line
		// gutter glyph: cursor line, selected line, both, or a plain space.
		gutter := " "
		switch {
		case i == headIdx && selected[i]:
			gutter = bothMarker
		case i == headIdx:
			gutter = cursorMarker
		case selected[i]:
			gutter = selMarker
		}
		shown = gutter + shown
		wrapped := ansi.Hardwrap(shown, w, true)
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(wrapped)
		// each '\n' in wrapped becomes a new visual row
		rows := strings.Count(wrapped, "\n") + 1
		for range rows {
			mapping = append(mapping, i)
		}
	}
	return b.String(), mapping
}

// renderTabs is the header: brand, chips, and a thin rule.
func (m *Model) renderTabs() string {
	chips := m.renderTabChips()
	left := renderBrand() + "  " + chips
	right := m.modeBadges()

	// 1-cell horizontal padding on both edges so content doesn't touch the
	// terminal walls.
	const hPad = 1
	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	gap := m.width - leftW - rightW - hPad*2
	if gap < 1 {
		gap = 1
	}
	pad := strings.Repeat(" ", hPad)
	bar := pad + left + strings.Repeat(" ", gap) + right + pad
	sep := lipgloss.NewStyle().Foreground(lipgloss.Color("237")).Render(strings.Repeat("─", m.width))
	return bar + "\n" + sep
}

func (m *Model) renderTabChips() string {
	var parts []string
	for i, name := range m.tabs {
		// color-code the label by service status: gray pending, red error,
		// green active. the all-tab has no status, so it stays neutral.
		fg := lipgloss.Color("250")
		if name != allTab {
			fg = statusColor(m.statuses[name])
		}
		var chip string
		if i == m.active {
			// active tab is a filled teal chip with white text; teal/emerald is the
			// shared accent for the current/selected thing across the UI.
			chip = lipgloss.NewStyle().
				Foreground(lipgloss.Color("231")).
				Background(lipgloss.Color(accentColor)).
				Bold(true).
				Padding(0, 1).
				Render(name)
		} else {
			// inactive tabs are plain text colored by status, no background.
			chip = lipgloss.NewStyle().
				Foreground(fg).
				Padding(0, 1).
				Render(name)
		}
		parts = append(parts, chip)
	}
	return strings.Join(parts, " ")
}

// statusColor maps a service status to the tab label color: green when active,
// red on error, gray while pending or otherwise idle.
func statusColor(status string) lipgloss.Color {
	switch status {
	case "running":
		return lipgloss.Color("82")
	case "crashed":
		return lipgloss.Color("203")
	default:
		return lipgloss.Color("244")
	}
}

// barBgColor is the background tint used for the bottom bar (footer).
const barBgColor = "236"

// barStyle returns the base style for a bottom-bar cell with the shared
// background tint. Foreground is set per-segment.
func barStyle() lipgloss.Style {
	return lipgloss.NewStyle().Background(lipgloss.Color(barBgColor))
}

// renderBar paints a left/center/right bar onto barBgColor across the full
// terminal width: a top rule plus a single content row. Empty segments collapse
// to spaces.
func (m *Model) renderBar(left, center, right string) string {
	bg := barStyle()
	// each spacer is rendered with the bar bg explicitly; otherwise the ANSI
	// resets inside the pre-styled segments would drop subsequent spaces back to
	// the terminal default, leaving gaps between the status chunks.
	gap := func(n int) string {
		if n <= 0 {
			return ""
		}
		return bg.Render(strings.Repeat(" ", n))
	}
	lw := lipgloss.Width(left)
	cw := lipgloss.Width(center)
	rw := lipgloss.Width(right)
	const hPad = 1
	usable := m.width - hPad*2
	if usable < 0 {
		usable = 0
	}
	rest := usable - lw - cw - rw
	if rest < 2 {
		rest = 2
	}
	var content string
	if cw == 0 {
		content = gap(hPad) + left + gap(rest) + right + gap(hPad)
	} else {
		gapL := rest / 2
		gapR := rest - gapL
		content = gap(hPad) + left + gap(gapL) + center + gap(gapR) + right + gap(hPad)
	}
	if w := lipgloss.Width(content); w < m.width {
		content += gap(m.width - w)
	}
	rule := lipgloss.NewStyle().Foreground(lipgloss.Color("237")).Render(strings.Repeat("─", m.width))
	return rule + "\n" + content
}

func (m *Model) renderFooter() string {
	dim := barStyle().Foreground(lipgloss.Color("244"))
	val := barStyle().Foreground(lipgloss.Color("250")).Bold(true)

	// left: selection shortcuts while in cursor mode; otherwise a single hint for
	// the key that enters it, so the feature is discoverable. The tab chips
	// already show the active service and its status dot.
	var left string
	if m.cursorMode {
		left = m.renderCursorHints(dim, val)
	} else {
		left = m.renderScrollHints(dim, val)
	}
	// container switcher hint, only on a tab that has children (docker). In the
	// footer so it never competes with the header tab chips.
	if nav := m.renderContainerNav(dim, val); nav != "" {
		if left != "" {
			left = nav + dim.Render("  ·  ") + left
		} else {
			left = nav
		}
	}

	// right: watch footprint, uptime + reload count for the active tab.
	right := m.renderRightFooter(dim, val)

	return m.renderBar(left, "", right)
}

// renderContainerNav renders the compact container indicator shown in the footer
// for a service tab with children (docker compose). Form "<key> <focus> i/n":
// the bound switch key, the focus ("all" or a container name), and ring
// position. Empty for tabs without children.
func (m *Model) renderContainerNav(dim, val lipgloss.Style) string {
	children := m.childList[m.activeTab()]
	if len(children) == 0 {
		return ""
	}
	focus := m.childFocus[m.activeTab()]
	label, idx := "all", 0
	if focus != "" {
		label = stripTabPrefix(focus, m.activeTab())
		for i, c := range children {
			if c == focus {
				idx = i + 1
				break
			}
		}
	}
	key := m.keyFor(control.ActionNextChild)
	// fixed-width label so the i/n counter keeps its column as containers change.
	return val.Render(key) + dim.Render(" ") + val.Render(fitLabel(label, containerLabelWidth)) +
		dim.Render(fmt.Sprintf(" %d/%d", idx+1, len(children)+1))
}

// containerLabelWidth is the column the container name occupies in the footer
// ring. Wide enough that most names fit without padding the counter off-screen,
// and fixed so the i/n counter stays put as the focused container changes.
const containerLabelWidth = 20

// stripTabPrefix drops a redundant "<tab>-"/"<tab>_" prefix from a container
// name: the tab already names the service, so "web-api" under the "web" tab
// shows as "api". A name equal to the tab (no remainder) is left as-is.
func stripTabPrefix(name, tab string) string {
	for _, sep := range []string{"-", "_"} {
		p := tab + sep
		if len(name) > len(p) && strings.HasPrefix(name, p) {
			return name[len(p):]
		}
	}
	return name
}

// fitLabel pads s with spaces to width w, or truncates it with a trailing "…"
// when it overflows, so a styled label always occupies exactly w columns.
func fitLabel(s string, w int) string {
	r := []rune(s)
	if len(r) > w {
		if w <= 1 {
			return string(r[:w])
		}
		return string(r[:w-1]) + "…"
	}
	return s + strings.Repeat(" ", w-len(r))
}

// renderCursorHints renders the selection-mode shortcut strip shown while cursor
// mode is active. Keys come from the live keymap; an unbound action is dropped.
// renderScrollHints is the footer's left hint row in scroll mode: the verbs that
// aren't already visible from the tab chips. Restart is dropped on the all-tab,
// where it is a no-op.
func (m *Model) renderScrollHints(dim, val lipgloss.Style) string {
	hints := []struct {
		action control.Action
		label  string
	}{
		{control.ActionCursorMode, "export lines"},
		{control.ActionRestart, "restart"},
		{control.ActionCommandCenter, "help"},
	}
	var parts []string
	for _, h := range hints {
		if h.action == control.ActionRestart && m.activeTab() == allTab {
			continue
		}
		key := m.keyFor(h.action)
		if key == "" {
			continue
		}
		parts = append(parts, val.Render(key)+dim.Render(" "+h.label))
	}
	return strings.Join(parts, dim.Render("  ·  "))
}

func (m *Model) renderCursorHints(dim, val lipgloss.Style) string {
	hints := []struct {
		action control.Action
		label  string
	}{
		{control.ActionToggleSelect, "select"},
		{control.ActionExtendDown, "extend"},
		{control.ActionCopy, "copy"},
		{control.ActionWriteSelection, "write"},
		{control.ActionAppendSelection, "append"},
		{control.ActionClearCursor, "exit"},
	}
	var parts []string
	for _, h := range hints {
		key := m.keyFor(h.action)
		if key == "" {
			continue
		}
		parts = append(parts, val.Render(key)+dim.Render(" "+h.label))
	}
	return strings.Join(parts, dim.Render("  ·  "))
}

// keyFor returns the first key bound to an action in the live keymap, or "".
// Keys are humanized (e.g. " " -> "space") for display.
func (m *Model) keyFor(a control.Action) string {
	for _, e := range m.keymap.Help() {
		if e.Action == a && len(e.Keys) > 0 {
			return humanizeKey(e.Keys[0])
		}
	}
	return ""
}

func (m *Model) renderRightFooter(dim, val lipgloss.Style) string {
	now := time.Now()
	url := barStyle().Foreground(lipgloss.Color("75"))
	tab := m.activeTab()
	if tab != allTab {
		var parts []string
		// while a container is focused, show that container's ports; the merged
		// view falls back to the service-level (aggregate) ports.
		if u := formatPortsURL(m.ports[m.viewKey()]); u != "" {
			parts = append(parts, url.Render(u))
		}
		if w := m.renderWatchStat(dim, val); w != "" {
			parts = append(parts, w)
		}
		if t, ok := m.startedAt[tab]; ok {
			parts = append(parts, dim.Render("↑ ")+dim.Render(formatUptime(now.Sub(t))))
		}
		if n := m.reloads[tab]; n > 0 {
			parts = append(parts, dim.Render("⟳ ")+val.Render(strconv.Itoa(n)))
		}
		return strings.Join(parts, dim.Render(" · "))
	}
	// all-tab: oldest uptime and total reloads across services.
	var oldest time.Time
	for _, s := range m.services {
		t, ok := m.startedAt[s]
		if !ok {
			continue
		}
		if oldest.IsZero() || t.Before(oldest) {
			oldest = t
		}
	}
	total := 0
	for _, n := range m.reloads {
		total += n
	}
	var parts []string
	if w := m.renderWatchStat(dim, val); w != "" {
		parts = append(parts, w)
	}
	if !oldest.IsZero() {
		parts = append(parts, dim.Render("↑ ")+dim.Render(formatUptime(now.Sub(oldest))))
	}
	if total > 0 {
		parts = append(parts, dim.Render("⟳ ")+val.Render(strconv.Itoa(total)))
	}
	return strings.Join(parts, dim.Render(" · "))
}

// renderWatchStat is the compact watch footprint shown left of the uptime:
// "watch <files>f <dirs>d". On the all-tab it shows the aggregate counts, on a
// service tab that service's own counts. Empty until the first WatchStatsMsg
// lands or when the active service has no watcher.
func (m *Model) renderWatchStat(dim, val lipgloss.Style) string {
	files, dirs := m.watchFiles, m.watchDirs
	if m.activeTab() != allTab {
		s, ok := m.watchPerSvc[m.activeTab()]
		if !ok {
			return ""
		}
		files, dirs = s.Files, s.Dirs
	}
	if files == 0 && dirs == 0 {
		return ""
	}
	return dim.Render("watch ") + val.Render(strconv.Itoa(files)) + dim.Render("f ") +
		val.Render(strconv.Itoa(dirs)) + dim.Render("d")
}

// formatPortsURL renders a service's listening ports as a loopback address:
// "http://127.0.0.1:8080" for one, "http://127.0.0.1:8080, 8081" for several.
// Empty for a service with no known port.
func formatPortsURL(ports []int) string {
	if len(ports) == 0 {
		return ""
	}
	nums := make([]string, len(ports))
	for i, p := range ports {
		nums[i] = strconv.Itoa(p)
	}
	return "http://127.0.0.1:" + strings.Join(nums, ", ")
}

// formatUptime renders a duration compactly (12s, 2m13s, 1h04m, 3d02h), trimmed
// to two significant units.
func formatUptime(d time.Duration) string {
	if d < time.Second {
		return "0s"
	}
	s := int64(d.Seconds())
	days := s / 86400
	hours := (s % 86400) / 3600
	mins := (s % 3600) / 60
	secs := s % 60
	switch {
	case days > 0:
		return fmt.Sprintf("%dd%02dh", days, hours)
	case hours > 0:
		return fmt.Sprintf("%dh%02dm", hours, mins)
	case mins > 0:
		return fmt.Sprintf("%dm%02ds", mins, secs)
	default:
		return fmt.Sprintf("%ds", secs)
	}
}

// modeBadges returns the corner pill cluster, rendering only the modes that are
// currently on.
func (m *Model) modeBadges() string {
	var pills []string
	if m.flash != "" && time.Since(m.flashAt) < flashDuration {
		pills = append(pills, badge(m.flash, m.flashColor))
	}
	if m.cursorMode {
		pills = append(pills, badge("SELECT", "214"))
	}
	if c := m.childFocus[m.activeTab()]; c != "" {
		pills = append(pills, badge(c, accentColor))
	}
	return strings.Join(pills, " ")
}

// setFlash arms a transient action badge (e.g. COPIED) in the corner.
func (m *Model) setFlash(text, color string) {
	m.flash = text
	m.flashColor = color
	m.flashAt = time.Now()
}

func badge(text, color string) string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("231")).
		Background(lipgloss.Color(color)).
		Padding(0, 1).
		Bold(true).
		Render(text)
}

func padRight(s string, n int) string {
	w := lipgloss.Width(s)
	if w >= n {
		return s
	}
	return s + strings.Repeat(" ", n-w)
}

// renderBrand draws the gradient "blink" wordmark in the header.
func renderBrand() string {
	letters := []struct {
		r rune
		c string
	}{
		{'b', "30"}, {'l', "36"}, {'i', "44"}, {'n', "51"}, {'k', "87"},
	}
	var out strings.Builder
	for _, l := range letters {
		out.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(l.c)).Bold(true).Render(string(l.r)))
	}
	return out.String()
}

// palette mirrors the plain UI's service colors for consistency.
var palette = []lipgloss.Color{
	lipgloss.Color("39"), lipgloss.Color("214"), lipgloss.Color("141"),
	lipgloss.Color("82"), lipgloss.Color("203"), lipgloss.Color("220"),
	lipgloss.Color("117"), lipgloss.Color("213"),
}

// tintPalette is the darkened counterpart of palette: one subdued background
// color per service slot, used only in the all-tab to hint which service a line
// belongs to without obscuring the text.
var tintPalette = []lipgloss.Color{
	lipgloss.Color("17"),  // dim blue, palette 39
	lipgloss.Color("58"),  // dim olive, 214
	lipgloss.Color("53"),  // dim purple, 141
	lipgloss.Color("22"),  // dim green, 82
	lipgloss.Color("52"),  // dim red, 203
	lipgloss.Color("100"), // dim yellow, 220
	lipgloss.Color("24"),  // dim teal, 117
	lipgloss.Color("89"),  // dim magenta, 213
}

func paletteIndex(name string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	return int(h.Sum32()) % len(palette)
}

func serviceStyle(name string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(palette[paletteIndex(name)]).Bold(true)
}

// serviceTintStyle returns the muted-background style used to tint a service's
// lines in the all-tab. Foreground inherits from the line content.
func serviceTintStyle(name string) lipgloss.Style {
	return lipgloss.NewStyle().Background(tintPalette[paletteIndex(name)])
}
