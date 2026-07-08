package ui

import (
	"bufio"
	"context"
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/term"
	"github.com/mattn/go-isatty"

	"github.com/toaweme/log"

	"github.com/toaweme/blink/blink/internal/theme"
	"github.com/toaweme/blink/core/addon"
	"github.com/toaweme/blink/core/config"
	"github.com/toaweme/blink/core/output"
	"github.com/toaweme/blink/core/supervisor"
)

const (
	plainBufferLines = 5000
	// refreshCooldown rate-limits the r redraw; pressing the key fast otherwise
	// floods the terminal with reflows.
	refreshCooldown = 700 * time.Millisecond
)

// Plain prints prefixed log lines from all services interleaved on stdout. It is
// the default when stdout is not a TTY. When stdin is a TTY (e.g. -u plain on a
// real terminal), it also listens for an r keypress to redraw buffered history.
type Plain struct {
	out io.Writer
	reg *addon.Registry

	mu  sync.Mutex
	sup *supervisor.Supervisor

	bufMu   sync.Mutex
	buffer  []string
	lastRef time.Time
}

var _ UserInterface = (*Plain)(nil)

// NewPlain returns a Plain UI that writes to stdout, backed by the given
// addon registry.
func NewPlain(reg *addon.Registry) *Plain {
	return &Plain{out: os.Stdout, reg: reg}
}

// PlainIsAppropriate returns true when output is being piped/redirected.
func PlainIsAppropriate() bool {
	return !isatty.IsTerminal(os.Stdout.Fd()) && !isatty.IsCygwinTerminal(os.Stdout.Fd())
}

// Run starts the supervisor and streams prefixed log lines to stdout, blocking
// until the event stream closes.
func (p *Plain) Run(cfg config.Config) error {
	sup, err := supervisor.New(cfg, p.reg)
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.sup = sup
	p.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// subscribe before Start so a fast service's boot-time status/log events
	// aren't dropped: the Hub only delivers to existing subscribers, and a
	// shell/go service can reach "running"/"crashed" before any consumer is
	// registered. The buffered channels latch the events. See blink.go.
	sub, cancelSub := sup.Subscribe()
	defer cancelSub()

	// log writing is a Hub subscriber independent of the rendered output, so
	// `blink run -u plain` still produces <LogDir>/<svc>.log when enabled.
	var logSub output.Subscription
	writeLog := cfg.LogWriteEnabled()
	if writeLog {
		if err := os.MkdirAll(cfg.Paths.LogDir, 0o755); err != nil {
			log.Warn("plain ui: failed to create log dir; log writing disabled", "path", cfg.Paths.LogDir, "error", err)
			writeLog = false
		} else {
			var cancelLogSub func()
			logSub, cancelLogSub = sup.Subscribe()
			defer cancelLogSub()
		}
	}

	if err := sup.Start(ctx); err != nil {
		return err
	}

	// raw-stdin reader for the r refresh, only when stdin is an interactive
	// terminal. Restoring cooked mode is critical; defer handles all exits.
	stopInput, raw := p.maybeStartInputLoop(ctx)
	defer stopInput()

	// raw mode clears the tty's OPOST/ONLCR, so the driver no longer turns LF
	// into CRLF and printed lines staircase. Translate it ourselves while raw,
	// but only when stdout is the terminal (piped output stays plain). Done
	// before the consumers start so emit() never races on p.out.
	if raw && isatty.IsTerminal(os.Stdout.Fd()) {
		p.out = &crlfWriter{w: os.Stdout}
	}

	if writeLog {
		sink := newLogSink(cfg.Paths.LogDir, true)
		go sink.consume(logSub)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		p.consumeEvents(sub)
	}()
	go p.consumeLogs(sub)

	<-done
	return nil
}

// consumeEvents prints status changes. The supervisor itself routes runner
// output through the hub now, so this UI no longer attaches log tees.
func (p *Plain) consumeEvents(sub output.Subscription) {
	for ev := range sub.Status {
		label := ev.Service
		if ev.Child != "" {
			label = ev.Service + "/" + ev.Child
		}
		p.emit(p.format(label, "[status] "+ev.Status))
		if ev.Err != "" {
			p.emit(p.format(label, "[error]  "+ev.Err))
		}
	}
}

// consumeLogs prints every captured line: shell-runtime output, docker
// compose container output, and anything else flowing through the hub.
func (p *Plain) consumeLogs(sub output.Subscription) {
	for ln := range sub.Logs {
		label := ln.Service
		if ln.Child != "" {
			label = ln.Service + "/" + ln.Child
		}
		p.emit(p.format(label, ln.Line))
	}
}

// Stop tears down the supervisor.
func (p *Plain) Stop(_ config.Config) error {
	p.mu.Lock()
	sup := p.sup
	p.mu.Unlock()
	if sup == nil {
		return nil
	}
	log.Info("stopping (plain UI)")
	return sup.Stop(context.Background())
}

func (p *Plain) format(label, line string) string {
	return serviceStyle(label).Render("["+label+"]") + " " + line
}

// emit prints a single rendered line and appends it to the in-memory buffer used
// by the r refresh. The buffer is a sliding window; old lines drop.
func (p *Plain) emit(line string) {
	fmt.Fprintln(p.out, line)
	p.bufMu.Lock()
	p.buffer = append(p.buffer, line)
	if len(p.buffer) > plainBufferLines {
		p.buffer = p.buffer[len(p.buffer)-plainBufferLines:]
	}
	p.bufMu.Unlock()
}

// refresh clears the screen and reprints every buffered line. Rate-limited so
// holding r can't pin the CPU on repainting.
func (p *Plain) refresh() {
	p.bufMu.Lock()
	if time.Since(p.lastRef) < refreshCooldown {
		p.bufMu.Unlock()
		return
	}
	p.lastRef = time.Now()
	lines := append([]string(nil), p.buffer...)
	p.bufMu.Unlock()

	// ESC[2J clears the screen, ESC[H homes the cursor: standard ANSI, works on
	// any vt100-compatible terminal.
	fmt.Fprint(p.out, "\x1b[2J\x1b[H")
	fmt.Fprintln(p.out, strings.Join(lines, "\n"))
}

// maybeStartInputLoop sets stdin to raw mode and spawns a goroutine that reacts
// to single-key inputs. Returns a cleanup func that restores cooked mode (caller
// must defer it) and whether raw mode was entered. When stdin isn't a TTY it is
// a no-op and reports false.
func (p *Plain) maybeStartInputLoop(ctx context.Context) (func(), bool) {
	fd := int(os.Stdin.Fd())
	if !isatty.IsTerminal(uintptr(fd)) {
		return func() {}, false
	}
	state, err := term.MakeRaw(uintptr(fd))
	if err != nil {
		// not fatal: just means no r refresh.
		log.Warn("plain ui: failed to put stdin in raw mode; refresh disabled", "error", err)
		return func() {}, false
	}
	r := bufio.NewReader(os.Stdin)
	//nolint:gosec // the goroutine's only context.Background use is the shutdown Stop below, which intentionally does not inherit the about-to-cancel run ctx.
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			b, err := r.ReadByte()
			if err != nil {
				return
			}
			switch b {
			case 'r', 'R':
				p.refresh()
			case 'q', 0x03: // q or ctrl-c
				p.mu.Lock()
				sup := p.sup
				p.mu.Unlock()
				if sup != nil {
					// detached on purpose: a quit-key shutdown must run its full
					// teardown even though the run ctx is about to be canceled.
					_ = sup.Stop(context.Background()) //nolint:contextcheck // shutdown must not inherit the about-to-cancel run ctx
				}
				return
			}
		}
	}()
	return func() { _ = term.Restore(uintptr(fd), state) }, true
}

// crlfWriter translates bare LF into CRLF. Plain mode puts the terminal in raw
// mode for single-key input, which clears the tty's OPOST/ONLCR output
// translation; without this every printed line would staircase. Writes are
// serialized so concurrent emitters never interleave a half-translated line.
type crlfWriter struct {
	mu sync.Mutex
	w  io.Writer
}

var _ io.Writer = (*crlfWriter)(nil)

func (c *crlfWriter) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	buf := make([]byte, 0, len(p)+8)
	for i, b := range p {
		if b == '\n' && (i == 0 || p[i-1] != '\r') {
			buf = append(buf, '\r')
		}
		buf = append(buf, b)
	}
	if _, err := c.w.Write(buf); err != nil {
		return 0, err
	}
	return len(p), nil
}

// serviceStyle colors a service name from the shared categorical palette,
// deterministically and stable across runs, so a service keeps the same color in
// both the plain and TUI renderers.
func serviceStyle(name string) lipgloss.Style {
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	c := theme.ServicePalette[int(h.Sum32())%len(theme.ServicePalette)]
	return lipgloss.NewStyle().Foreground(c).Bold(true)
}
