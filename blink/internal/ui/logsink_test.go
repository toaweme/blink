package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/toaweme/blink/core/output"
	"github.com/toaweme/blink/core/protocol"
)

// feed runs the given lines through a sink and returns once the sink has
// drained them (the channel is closed, so consume returns).
func feed(sink *logSink, lines ...protocol.LogLine) {
	ch := make(chan protocol.LogLine, len(lines))
	for _, ln := range lines {
		ch <- ln
	}
	close(ch)
	sink.consume(output.Subscription{Logs: ch})
}

func Test_LogSink_WritesPerService(t *testing.T) {
	dir := t.TempDir()
	sink := newLogSink(dir, true)
	feed(sink,
		protocol.LogLine{Service: "web", Line: "hello"},
		protocol.LogLine{Service: "api", Line: "world"},
		protocol.LogLine{Service: "web", Child: "child", Line: "nested"},
	)

	web, err := os.ReadFile(filepath.Join(dir, "web.log"))
	if err != nil {
		t.Fatalf("read web.log: %v", err)
	}
	if !strings.Contains(string(web), "hello") || !strings.Contains(string(web), "[child] nested") {
		t.Fatalf("web.log = %q", web)
	}
	if _, err := os.Stat(filepath.Join(dir, "api.log")); err != nil {
		t.Fatalf("api.log not written: %v", err)
	}
}

func Test_LogSink_DisabledWritesNothing(t *testing.T) {
	dir := t.TempDir()
	sink := newLogSink(dir, false)
	feed(sink, protocol.LogLine{Service: "web", Line: "hello"})

	if _, err := os.Stat(filepath.Join(dir, "web.log")); !os.IsNotExist(err) {
		t.Fatalf("disabled sink should write nothing, stat err = %v", err)
	}
}

func Test_LogSink_Toggle(t *testing.T) {
	sink := newLogSink(t.TempDir(), false)
	if sink.Enabled() {
		t.Fatalf("sink should start disabled")
	}
	if !sink.Toggle() || !sink.Enabled() {
		t.Fatalf("Toggle should enable and report true")
	}
	if sink.Toggle() || sink.Enabled() {
		t.Fatalf("second Toggle should disable and report false")
	}
}
