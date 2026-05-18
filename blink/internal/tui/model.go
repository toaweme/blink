package tui

import (
	"fmt"
	"hash/fnv"
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

// Controller is the session-action sink the TUI dispatches into. The same
// path serves both the local supervisor (controllerAdapter) and a remote
// mirror (mirrorController over session.Client); the model never special-
// cases which. Only role-allowed session actions reach here; view actions
// stay in the model.
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

	tabs   []string
	active int

	vp      viewport.Model
	width   int
	height  int
	keymap  control.Keymap
	control Controller
	ready   bool

	followTail bool

	// per-tab scroll state so switching tabs returns to where the user was.
	scrollState map[string]tabScroll

	// modalScroll is the line offset for the command-center modal, used
	// when the modal body is taller than the terminal. Reset to 0 when
	// the modal opens; j/k or up/down adjusts.
	modalScroll int

	// helpOpen is true while the command-center/help modal is visible.
	// Opened with / or ?, closed with esc/q.
	helpOpen bool

	// rawMode (toggled with z) tears down the TUI overlay: bubbletea exits
	// alt-screen, mouse capture is disabled, View() returns empty, and new
	// lines stream via tea.Println into the native terminal scrollback.
	// This is the mode where native scroll + mouse-select Just Work.
	rawMode bool

	// animation
	spinner    spinner.Model
	pulsePhase int

	// wrappedToBuffer[i] is the buffer index that wrapped visual row i was
	// emitted from. Populated by renderBufferMapped on every refresh; used to
	// keep the cursor row visible while scrolling.
	wrappedToBuffer []int

	// startedAt[svc] is when the service most recently transitioned into
	// "running". Cleared on any non-running status. Drives the footer
	// uptime readout.
	startedAt map[string]time.Time

	// reloads[svc] counts how many times the service has been restarted
	// since the TUI started (incremented on each "restarting"/"building"
	// → "running" cycle, starting from 0 on the first run).
	reloads map[string]int

	// watchFiles + watchDirs are the latest aggregate counts published
	// by the supervisor via WatchStatsMsg. Both 0 before the first msg.
	watchFiles  int
	watchDirs   int
	watchPerSvc map[string]WatchStat

	// cursorMode gates the line cursor + selection. Off by default: ↑/↓
	// scroll the viewport fast. Toggled with `e`; on, ↑/↓ move the cursor
	// and the selection keys (space, shift+↑/↓) become live.
	cursorMode bool
	// tabCursor[tab] is the buffer index the cursor is parked on for that
	// tab. Defaults to the last line (tail) for new tabs - see ensureCursor.
	tabCursor map[string]int
	// selected[tab] is the set of buffer indices selected on that tab. The
	// set allows gaps (space toggles single lines; shift+↑/↓ extends a run).
	// Selection is stateless: there is no anchor, only the rows in this set.
	// Cleared when cursor mode exits.
	selected map[string]map[int]bool

	// log sink controls, injected by the host UI via WithLogControl. logDir
	// is where selection writes land (<svc>.selected.log); logsOn mirrors the
	// sink state for the footer; logToggle flips the sink live (nil = no-op,
	// e.g. a remote mirror).
	logDir    string
	logsOn    bool
	logToggle func() bool

	// flash is a transient badge (e.g. COPIED, WRITTEN) shown in the top-right
	// for flashDuration after an action. The pulse tick keeps re-rendering, so
	// it fades on its own without an explicit timer.
	flash      string
	flashColor string
	flashAt    time.Time

	// watchHintAt is when the watch-stats hint was last armed (a tab switch).
	// The center footer shows "watching N files, M dirs" only for
	// watchHintDuration after a switch, then it fades - the heartbeat tick
	// re-renders it away, same as flash. Permanent watch counts cluttered the
	// bar without earning the space.
	watchHintAt time.Time
}

// flashDuration is how long a transient action badge (COPIED/WRITTEN) stays
// visible before the next render drops it.
const flashDuration = 1500 * time.Millisecond

// watchHintDuration is how long the watch-stats hint stays in the footer
// after a tab switch before the heartbeat tick fades it.
const watchHintDuration = 3 * time.Second

// tabScroll captures where the viewport was sitting for a given tab so we can
// restore it when the user comes back. followTail collapses to "stick to the
// bottom" - if it's set we ignore yOffset and just GotoBottom on restore.
type tabScroll struct {
	yOffset    int
	followTail bool
}

func NewModel(services []string, ctrl Controller) Model {
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

	return Model{
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
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, tickCmd())
}

// Update routes incoming messages to per-type handlers. Heavy logic
// lives in dedicated helpers (handleKey + per-modal subfunctions) so
// this dispatcher stays scannable.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		// fired after exitRawMode reattaches the alt-screen; bring the
		// viewport back to whatever offset it had before zen.
		m.restoreScroll()
		return m, nil
	case WatchStatsMsg:
		// flash the counts once when they first become known, so the user
		// sees the watch footprint on startup without having to switch tabs.
		if m.watchPerSvc == nil {
			m.watchHintAt = time.Now()
		}
		m.watchFiles = msg.Files
		m.watchDirs = msg.Dirs
		m.watchPerSvc = msg.PerSvc
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case tea.MouseMsg:
		// horizontal wheel switches tabs (trackpad two-finger swipe,
		// shift+wheel on most mice). Fires on press only to dodge
		// rapid-fire double events.
		if msg.Action == tea.MouseActionPress {
			switch msg.Button {
			case tea.MouseButtonWheelLeft:
				m.saveScroll()
				m.active = (m.active - 1 + len(m.tabs)) % len(m.tabs)
				m.restoreScroll()
				return m, nil
			case tea.MouseButtonWheelRight:
				m.saveScroll()
				m.active = (m.active + 1) % len(m.tabs)
				m.restoreScroll()
				return m, nil
			}
		}
		// in export (cursor) mode the vertical wheel moves the cursor and
		// the viewport follows, so the cursor stays put under the scroll;
		// otherwise it falls through to the viewport for plain scrolling.
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
			}
		}
		// the vertical wheel falls through to the viewport so scrolling
		// works. Left-click is intentionally not bound: mouse reporting is
		// unreliable across terminals (e.g. JetBrains), so the line cursor
		// is keyboard-driven (`e` then ↑/↓).
	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	m.followTail = m.vp.AtBottom()
	return m, cmd
}

func (m Model) handleWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
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

func (m Model) handleLineMsg(msg LineMsg) (tea.Model, tea.Cmd) {
	line := msg.Line
	if msg.Child != "" {
		line = lipgloss.NewStyle().Faint(true).Render("["+msg.Child+"]") + " " + line
	}
	m.appendLine(msg.Service, line)
	if m.rawMode {
		// stream into the native terminal so the user can scroll/select.
		prefix := serviceStyle(msg.Service).Render("[" + msg.Service + "]")
		return m, tea.Println(prefix + " " + line)
	}
	if m.activeTab() == allTab || m.activeTab() == msg.Service {
		m.refreshViewportFollow()
	}
	return m, nil
}

func (m Model) handleStatusMsg(msg StatusMsg) (tea.Model, tea.Cmd) {
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
	if msg.Child != "" {
		label = "── " + msg.Child + ": " + msg.Status + " ──"
	}
	labelLine := lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render(label)
	m.appendLine(msg.Service, labelLine)
	if m.rawMode {
		prefix := serviceStyle(msg.Service).Render("[" + msg.Service + "]")
		return m, tea.Println(prefix + " " + labelLine)
	}
	if m.activeTab() == allTab || m.activeTab() == msg.Service {
		m.refreshViewportFollow()
	}
	return m, nil
}

// handleKey is the top-level key dispatcher. Modal-scoped overlays
// (command-center, raw-mode) get first refusal; whatever they
// don't consume falls through to the global keymap.
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.helpOpen {
		return m.handleCommandCenterKey(msg)
	}
	if m.rawMode {
		return m.handleRawModeKey(msg)
	}
	return m.handleGlobalKey(msg)
}

// handleCommandCenterKey owns the / and ? command-center overlay:
// tab cycling and modal scroll.
func (m Model) handleCommandCenterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if a, ok := m.keymap.Lookup(msg.String()); ok && a == control.ActionCommandCenter {
		m.helpOpen = false
		return m, nil
	}
	switch {
	case msg.String() == "esc", msg.String() == "q":
		m.helpOpen = false
		return m, nil
	case msg.String() == "ctrl+c":
		// q is swallowed while the help modal is open (it's an
		// easy fat-finger when reading); ctrl+c still tears down.
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

// handleRawModeKey is the minimal keymap that works while the TUI has
// handed the screen back to the user. Only z (exit zen) and quit
// respond; everything else passes through to the native terminal.
func (m Model) handleRawModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch a, _ := m.keymap.Lookup(msg.String()); a {
	case control.ActionQuit:
		return m, tea.Quit
	case control.ActionToggleZen:
		return m, m.exitRawMode()
	}
	return m, nil
}

// scrollStep is how many lines ↑/↓ move the viewport in scroll mode. Bigger
// than one line so paging through logs feels fast, small enough to stay
// readable.
const scrollStep = 3

// handleGlobalKey is the main keymap: tab navigation, restart, action
// verbs. Reached when no modal owns the input.
func (m Model) handleGlobalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// fixed scroll navigation, outside the rebindable keymap, so moving
	// through logs is always fast regardless of mode. In export (cursor)
	// mode these drive the cursor and the viewport follows it, so the
	// cursor never strands off-screen; in scroll mode they move the
	// viewport directly.
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
			m.jumpCursorTo(len(m.buffers[m.activeTab()]) - 1)
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
		m.saveScroll()
		m.active = (m.active + 1) % len(m.tabs)
		m.restoreScroll()
		return m, nil
	case control.ActionPrevTab:
		m.saveScroll()
		m.active = (m.active - 1 + len(m.tabs)) % len(m.tabs)
		m.restoreScroll()
		return m, nil
	case control.ActionRestart:
		if name := m.activeTab(); name != allTab && m.control != nil {
			_ = m.control.Dispatch(control.ActionRestart, name)
		}
		return m, nil
	case control.ActionInsertBlank:
		// inject a blank spacer into the focused service's output. It flows
		// back through the Hub, so it lands in the buffer and the .log file;
		// no local append here or it would double. No-op on the all-tab.
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
			if idx < len(m.tabs) && idx != m.active {
				m.saveScroll()
				m.active = idx
				m.restoreScroll()
			}
		}
	}
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	m.followTail = m.vp.AtBottom()
	return m, cmd
}

// handleClear empties the active tab's buffer. Terminal-only: never
// touches disk.
func (m Model) handleClear() (tea.Model, tea.Cmd) {
	m.clearTab(m.activeTab())
	m.refreshViewport()
	return m, nil
}

// handleClearAll empties every buffer. Terminal-only: never touches
// disk.
func (m Model) handleClearAll() (tea.Model, tea.Cmd) {
	for tab := range m.buffers {
		m.buffers[tab] = nil
	}
	m.refreshViewport()
	return m, nil
}

// enterRawMode tears down the bubbletea overlay so the user can use the
// terminal's native scroll + mouse-select. Order matters here: tea.Println
// is silently dropped while the alt-screen is active, so we sequence
// ExitAltScreen → DisableMouse → Println(buffer) instead of batching.
func (m *Model) enterRawMode() tea.Cmd {
	m.saveScroll()
	m.rawMode = true
	// release mouse capture so native terminal scroll + select work.
	cmds := []tea.Cmd{tea.ExitAltScreen, tea.DisableMouse}
	// flush current buffer into the native scrollback so the user lands on
	// real content instead of an empty screen.
	if buf := m.buffers[m.activeTab()]; len(buf) > 0 {
		cmds = append(cmds, tea.Println(strings.Join(buf, "\n")))
	}
	return tea.Sequence(cmds...)
}

// exitRawMode brings the overlay back. Mouse capture is re-enabled only if
// the user had explicitly opted into it.
func (m *Model) exitRawMode() tea.Cmd {
	m.rawMode = false
	// re-capture the mouse (cell motion is on for the whole TUI lifetime).
	cmds := []tea.Cmd{tea.EnterAltScreen, tea.EnableMouseCellMotion}
	// schedule a refresh after re-entering the alt screen.
	cmds = append(cmds, func() tea.Msg { return refreshMsg{} })
	return tea.Batch(cmds...)
}

// refreshMsg is a no-op message we dispatch after re-entering alt-screen so
// the next Update tick re-renders the active tab.
type refreshMsg struct{}

// saveScroll snapshots where the active tab's viewport is parked.
func (m *Model) saveScroll() {
	if !m.ready {
		return
	}
	m.scrollState[m.activeTab()] = tabScroll{
		yOffset:    m.vp.YOffset,
		followTail: m.followTail,
	}
}

// restoreScroll repositions the viewport to wherever the now-active tab was
// last seen. Brand-new tabs default to tail-following.
func (m *Model) restoreScroll() {
	// every tab switch funnels through here, so arm the watch-stats hint
	// for the now-active tab. ready is irrelevant to the hint timer.
	m.watchHintAt = time.Now()
	if !m.ready {
		return
	}
	// rerender for the now-active tab before repositioning.
	content, mapping := m.renderBufferMapped(m.buffers[m.activeTab()])
	m.wrappedToBuffer = mapping
	m.vp.SetContent(content)
	s, ok := m.scrollState[m.activeTab()]
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
// accounting for the current layout, the modal, and the one-line top
// padding View() prepends for visual breathing room.
func (m Model) viewportSize() (int, int) {
	h := m.height - lipgloss.Height(m.renderTabs()) - lipgloss.Height(m.renderFooter()) - topPaddingLines
	if h < 1 {
		h = 1
	}
	return m.width, h
}

// topPaddingLines is the number of blank lines View() emits before the
// header. Kept as a const so every height calculation that subtracts
// chrome stays consistent.
const topPaddingLines = 1

func (m Model) View() string {
	if !m.ready {
		return "starting blink..."
	}
	// raw mode: yield the screen entirely so native scroll + mouse-select work.
	// new lines are pushed via tea.Println into the main screen buffer.
	if m.rawMode {
		return ""
	}
	if m.helpOpen {
		return m.renderHelpDialog()
	}
	footer := m.renderFooter()
	return strings.Repeat("\n", topPaddingLines) + m.renderTabs() + "\n" + m.vp.View() + "\n" + footer
}

func (m Model) activeTab() string { return m.tabs[m.active] }

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
	// In the all-tab the line gets a per-service background tint so the
	// reader's eye can group consecutive lines by service without
	// re-parsing the prefix on each one. Subtle enough that single
	// lines don't shout.
	tinted := serviceTintStyle(service).Render(prefix + line)
	all := append(m.buffers[allTab], tinted)
	if len(all) > maxBufferedLines {
		all = all[len(all)-maxBufferedLines:]
	}
	m.buffers[allTab] = all
}

// refreshViewport rerenders the active tab's content into the viewport while
// strictly preserving the user's scroll offset. Auto-tail-following on new
// lines is opt-in via refreshViewportFollow; everything else (mouse
// selection, status updates that aren't on the current tab, etc.) MUST
// go through refreshViewport so it never moves the user.
func (m *Model) refreshViewport() {
	if !m.ready {
		return
	}
	offset := m.vp.YOffset
	content, mapping := m.renderBufferMapped(m.buffers[m.activeTab()])
	m.wrappedToBuffer = mapping
	m.vp.SetContent(content)
	m.vp.SetYOffset(offset)
}

// refreshViewportFollow is the new-line variant: rerenders and, if the user
// was already parked at the tail, snaps to the new bottom.
func (m *Model) refreshViewportFollow() {
	if !m.ready {
		return
	}
	follow := m.followTail
	offset := m.vp.YOffset
	content, mapping := m.renderBufferMapped(m.buffers[m.activeTab()])
	m.wrappedToBuffer = mapping
	m.vp.SetContent(content)
	if follow {
		m.vp.GotoBottom()
	} else {
		m.vp.SetYOffset(offset)
	}
}

// renderBufferMapped renders the active buffer with the cursor gutter
// and selection tint, and returns a parallel slice whose i-th entry is
// the original buffer index that the i-th wrapped visual row came from.
// The mapping is what makes mouse hit-testing possible.
func (m Model) renderBufferMapped(lines []string) (string, []int) {
	if len(lines) == 0 {
		return "", nil
	}
	w := m.vp.Width
	if w <= 0 {
		w = 80
	}
	// the cursor and selection only render in cursor mode; scroll mode shows
	// a clean buffer with no gutter markers.
	headIdx := -1
	var selected map[int]bool
	if m.cursorMode {
		headIdx = m.cursorAt()
		selected = m.selected[m.activeTab()]
	}
	// all three gutter markers use the same left-edge glyph so the cursor and
	// selection bars line up exactly; only the color differs (cursor, selected,
	// both). A right-aligned glyph here would read as shifted one cell over.
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
		for r := 0; r < rows; r++ {
			mapping = append(mapping, i)
		}
	}
	return b.String(), mapping
}

// renderTabs is the header: brand + chips + thin rule.
func (m Model) renderTabs() string {
	chips := m.renderTabChips()
	left := renderBrand() + "  " + chips
	right := m.modeBadges()

	// 1-cell horizontal padding on both edges so the brand and the
	// UNLICENSED chip don't kiss the terminal walls.
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

func (m Model) renderTabChips() string {
	var parts []string
	for i, name := range m.tabs {
		var chip string
		if i == m.active {
			// active tab uses a flat filled chip (no rounded caps).
			chip = lipgloss.NewStyle().
				Foreground(lipgloss.Color("231")).
				Background(lipgloss.Color("36")).
				Bold(true).
				Padding(0, 1).
				Render(name)
		} else {
			// inactive tabs are plain text - no background, no rounded ends.
			chip = lipgloss.NewStyle().
				Foreground(lipgloss.Color("250")).
				Padding(0, 1).
				Render(name)
		}
		// the status dot lives outside the chip so its own ANSI color reset
		// doesn't punch a hole through the chip's background fill.
		if name != allTab {
			chip = m.statusDot(m.statuses[name]) + chip
		}
		parts = append(parts, chip)
	}
	return strings.Join(parts, " ")
}

// barBgColor is the slim background tint used for every interchangeable
// bottom-bar (currently just the footer). It renders into a fixed slot
// so a swap doesn't reflow the layout.
const barBgColor = "236"

// barStyle returns the base style for a bottom-bar cell: 1-cell side
// padding, the shared background tint. Foreground is set per-segment.
func barStyle() lipgloss.Style {
	return lipgloss.NewStyle().Background(lipgloss.Color(barBgColor))
}

// renderBar paints a left/center/right tripartite bar onto barBgColor
// across the full terminal width: a top rule plus a single content
// row. Empty segments collapse to spaces.
func (m Model) renderBar(left, center, right string) string {
	bg := barStyle()
	// each spacer is rendered with the bar bg explicitly, otherwise the
	// ANSI resets inside the pre-styled left/center/right segments would
	// drop subsequent spaces back to the terminal default (typically
	// black), leaving black gaps between the status chunks.
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

func (m Model) renderFooter() string {
	dim := barStyle().Foreground(lipgloss.Color("244"))
	val := barStyle().Foreground(lipgloss.Color("250")).Bold(true)
	accent := barStyle().Foreground(lipgloss.Color("82"))

	// left: selection shortcuts while in cursor mode; otherwise a single hint
	// for the key that enters it, so the selection feature is discoverable. The
	// service name / status / logs indicator that used to live here shifted
	// the whole bar on every tab switch (the name width varied), so it's gone -
	// the tab chips already show the active service and its status dot.
	var left string
	if m.cursorMode {
		left = m.renderCursorHints(dim, val)
	} else if key := m.keyFor(control.ActionCursorMode); key != "" {
		left = val.Render(key) + dim.Render(" export lines")
	}

	// center: watch stats, shown only for a moment after a tab switch then
	// faded - permanent counts cluttered the bar without earning it. On the
	// all-tab show aggregates; on a service tab show that service's watcher
	// counts. Hidden until the first WatchStatsMsg lands (or when the active
	// service has no watcher).
	var center string
	if time.Since(m.watchHintAt) < watchHintDuration {
		files, dirs := m.watchFiles, m.watchDirs
		if m.activeTab() != allTab {
			if s, ok := m.watchPerSvc[m.activeTab()]; ok {
				files, dirs = s.Files, s.Dirs
			} else {
				files, dirs = 0, 0
			}
		}
		if files > 0 || dirs > 0 {
			center = dim.Render("watching ") +
				val.Render(fmt.Sprintf("%d", files)) + dim.Render(" files, ") +
				val.Render(fmt.Sprintf("%d", dirs)) + dim.Render(" dirs")
		}
	}

	// right: uptime + reload count for the active tab.
	right := m.renderRightFooter(dim, val, accent)

	return m.renderBar(left, center, right)
}

// renderCursorHints renders the selection-mode shortcut strip shown in the
// footer center while cursor mode is active. Keys come from the live keymap so
// rebinds are reflected; an unbound action is dropped from the strip.
func (m Model) renderCursorHints(dim, val lipgloss.Style) string {
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
func (m Model) keyFor(a control.Action) string {
	for _, e := range m.keymap.Help() {
		if e.Action == a && len(e.Keys) > 0 {
			return humanizeKey(e.Keys[0])
		}
	}
	return ""
}

func (m Model) renderRightFooter(dim, val, accent lipgloss.Style) string {
	now := time.Now()
	tab := m.activeTab()
	if tab != allTab {
		var parts []string
		if t, ok := m.startedAt[tab]; ok {
			parts = append(parts, dim.Render("↑ ")+accent.Render(formatUptime(now.Sub(t))))
		}
		if n := m.reloads[tab]; n > 0 {
			parts = append(parts, dim.Render("⟳ ")+val.Render(fmt.Sprintf("%d", n)))
		}
		return strings.Join(parts, dim.Render(" · "))
	}
	// all-tab: aggregate "oldest uptime" + total reloads across services.
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
	if !oldest.IsZero() {
		parts = append(parts, dim.Render("↑ ")+accent.Render(formatUptime(now.Sub(oldest))))
	}
	if total > 0 {
		parts = append(parts, dim.Render("⟳ ")+val.Render(fmt.Sprintf("%d", total)))
	}
	return strings.Join(parts, dim.Render(" · "))
}

// formatUptime renders a duration as a compact human string: 12s,
// 2m13s, 1h04m, 3d02h. Trims to two significant units.
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

// modeBadges returns the small pill cluster that lives in the corner of the
// chrome. Only renders pills for modes that are currently on.
func (m Model) modeBadges() string {
	var pills []string
	if m.flash != "" && time.Since(m.flashAt) < flashDuration {
		pills = append(pills, badge(m.flash, m.flashColor))
	}
	if m.cursorMode {
		pills = append(pills, badge("SELECT", "214"))
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
		Foreground(lipgloss.Color("232")).
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

// statusDot picks a glyph and color per status. Transient statuses use the
// animated spinner frame; running uses a soft pulse so it feels alive.
func (m Model) statusDot(status string) string {
	switch status {
	case "running":
		// pulse between two close shades of green every tick
		shades := []lipgloss.Color{"82", "118", "82", "46"}
		c := shades[m.pulsePhase%len(shades)]
		return lipgloss.NewStyle().Foreground(c).Render("●")
	case "building", "restarting":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Render(m.spinner.View())
	case "crashed":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Render("✖")
	case "exited", "stopped":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render("○")
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("·")
	}
}

// renderBrand draws a tasteful gradient "blink" wordmark in the header.
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

// service color palette mirrors plain UI for consistency.
var palette = []lipgloss.Color{
	lipgloss.Color("39"), lipgloss.Color("214"), lipgloss.Color("141"),
	lipgloss.Color("82"), lipgloss.Color("203"), lipgloss.Color("220"),
	lipgloss.Color("117"), lipgloss.Color("213"),
}

// tintPalette is the darkened counterpart of palette - one very subdued
// background color per service slot. Hand-picked from xterm-256 so each
// stays distinct under most terminal themes. Used only in the all-tab
// to hint visually which service a line belongs to without obscuring
// the line text. (Most terminals render these so faintly that on a dark
// theme it looks like a 5% tint; on a light theme the row briefly
// brightens. Either way the effect is "subtle stripe", not "highlight".)
var tintPalette = []lipgloss.Color{
	lipgloss.Color("17"),  // dim blue ↔ palette 39
	lipgloss.Color("58"),  // dim olive ↔ 214
	lipgloss.Color("53"),  // dim purple ↔ 141
	lipgloss.Color("22"),  // dim green ↔ 82
	lipgloss.Color("52"),  // dim red ↔ 203
	lipgloss.Color("100"), // dim yellow ↔ 220
	lipgloss.Color("24"),  // dim teal ↔ 117
	lipgloss.Color("89"),  // dim magenta ↔ 213
}

func paletteIndex(name string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	return int(h.Sum32()) % len(palette)
}

func serviceStyle(name string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(palette[paletteIndex(name)]).Bold(true)
}

// serviceTintStyle returns the muted-background style used to tint a
// service's lines in the all-tab. Foreground inherits from the line
// content, so the existing serviceStyle prefix renders on top fine.
func serviceTintStyle(name string) lipgloss.Style {
	return lipgloss.NewStyle().Background(tintPalette[paletteIndex(name)])
}
