package ui

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/toaweme/blink/core/output"
	"github.com/toaweme/log"
)

// logSink writes captured service output to per-service files under a log
// directory. It is a plain Hub subscriber, independent of which UI renders:
// the blink TUI, the plain UI, and headless mode all attach one. Writing is
// gated by an atomic flag so it can be toggled live (the TUI `L` key) without
// tearing down the subscription. A disabled sink keeps draining its channel
// and drops every line, so a slow or paused sink never stalls the hub.
type logSink struct {
	logDir  string
	enabled atomic.Bool

	mu    sync.Mutex
	files map[string]*os.File
}

// newLogSink builds a sink rooted at logDir with the given initial state.
func newLogSink(logDir string, enabled bool) *logSink {
	s := &logSink{logDir: logDir, files: map[string]*os.File{}}
	s.enabled.Store(enabled)
	return s
}

// Enabled reports whether the sink is currently writing.
func (s *logSink) Enabled() bool { return s.enabled.Load() }

// Toggle flips the enabled flag and returns the new state. Used by the TUI
// `L` keybinding to turn log writing on/off mid-run.
func (s *logSink) Toggle() bool {
	on := !s.enabled.Load()
	s.enabled.Store(on)
	return on
}

// consume drains the subscription, writing each line while enabled. It blocks
// until the hub closes the channel (supervisor shutdown), then closes files.
func (s *logSink) consume(sub output.Subscription) {
	defer s.close()
	for ln := range sub.Logs {
		if !s.enabled.Load() {
			continue
		}
		f := s.fileFor(ln.Service)
		if f == nil {
			continue
		}
		line := ln.Line
		if ln.Child != "" {
			line = "[" + ln.Child + "] " + line
		}
		_, _ = f.WriteString(line + "\n")
	}
}

// fileFor lazily opens (append mode) the per-service log file. A failed open
// is logged once and that service's lines are dropped thereafter.
func (s *logSink) fileFor(service string) *os.File {
	s.mu.Lock()
	defer s.mu.Unlock()
	if f, ok := s.files[service]; ok {
		return f
	}
	if err := os.MkdirAll(s.logDir, 0o755); err != nil {
		log.Warn("log sink: failed to create log dir", "path", s.logDir, "error", err)
		s.files[service] = nil
		return nil
	}
	path := filepath.Join(s.logDir, service+".log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		log.Warn("log sink: failed to open service log", "service", service, "path", path, "error", err)
		s.files[service] = nil // remember the failure so we don't retry every line
		return nil
	}
	s.files[service] = f
	return f
}

func (s *logSink) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, f := range s.files {
		if f != nil {
			_ = f.Close()
		}
	}
}
