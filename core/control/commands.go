// Package control defines the typed admin-verb catalog that flows client to
// server inside a session, regardless of transport (unix socket, relay, WebRTC,
// Tailscale tunnel). Streaming the other way (StatusEvent, LogLine,
// ConfigSnapshot) lives in core/protocol; this package is the write side.
//
// One struct per verb. Adding a verb means a struct here, a wire helper in
// wire.go, and a handler in whichever module owns the side effect.
package control

import "github.com/toaweme/blink/core/protocol"

// VerbList and the other verbs identify a command on the wire. Stable strings,
// not iota, so older clients keep working when new verbs land.
const (
	VerbList         = "list"
	VerbRestart      = "restart"
	VerbSend         = "send"
	VerbSignal       = "signal"
	VerbDumpLogs     = "dump-logs"
	VerbPause        = "pause"
	VerbResume       = "resume"
	VerbReloadConfig = "reload-config"
	// VerbResync asks the host to re-emit a service's captured log buffer. Used
	// by mirror viewers to refresh their local view after a reconnect or when
	// the relay's snapshot ring rolled over older lines. Service="" means every
	// service.
	VerbResync = "resync"
)

// Command is the interface every admin verb implements. The concrete type
// carries the typed payload; Verb() returns the wire discriminator the
// dispatcher routes on.
type Command interface {
	Verb() string
}

// List enumerates supervised services and their current state.
type List struct{}

// Restart asks the supervisor to restart a service (or every service if
// Service is empty).
type Restart struct {
	Service string `json:"service,omitempty"`
}

// Send writes bytes to a service's stdin. Equivalent to typing into the
// running process's terminal.
type Send struct {
	Service string `json:"service"`
	Data    string `json:"data"`
}

// Signal delivers an OS signal to a service's process group.
type Signal struct {
	Service string `json:"service"`
	Signal  string `json:"signal"`
}

// DumpLogs writes a service's in-memory log buffer to a file at Path. Empty
// Path means "<DirRoot>/<service>.log" on the server side. LineNumbers prepends
// `cat -n`-style gutters for stable line references.
type DumpLogs struct {
	Service     string `json:"service"`
	Path        string `json:"path,omitempty"`
	LineNumbers bool   `json:"line_numbers,omitempty"`
}

// Pause stops the watcher for a service so file events no longer trigger a
// reload. Currently NotImplemented until the watcher grows a pause surface.
type Pause struct {
	Service string `json:"service"`
}

// Resume re-enables the watcher for a service paused via Pause.
type Resume struct {
	Service string `json:"service"`
}

// ReloadConfig asks the server to re-read blink.yaml and apply changes hot.
// Currently NotImplemented; the wire shape exists ahead of the supervisor
// learning to diff configs.
type ReloadConfig struct{}

// Resync asks the host to return one service's full captured log buffer (or
// every service when Service==""). The response rides in Result.Lines or
// Result.LinesByService so a viewer can replace its local buffer in one shot.
type Resync struct {
	Service string `json:"service,omitempty"`
}

// Verb returns the wire verb for List.
func (List) Verb() string { return VerbList }

// Verb returns the wire verb for Restart.
func (Restart) Verb() string { return VerbRestart }

// Verb returns the wire verb for Send.
func (Send) Verb() string { return VerbSend }

// Verb returns the wire verb for Signal.
func (Signal) Verb() string { return VerbSignal }

// Verb returns the wire verb for DumpLogs.
func (DumpLogs) Verb() string { return VerbDumpLogs }

// Verb returns the wire verb for Pause.
func (Pause) Verb() string { return VerbPause }

// Verb returns the wire verb for Resume.
func (Resume) Verb() string { return VerbResume }

// Verb returns the wire verb for ReloadConfig.
func (ReloadConfig) Verb() string { return VerbReloadConfig }

// Verb returns the wire verb for Resync.
func (Resync) Verb() string { return VerbResync }

// Result is the typed response a Dispatcher returns. Most verbs use only Ok and
// Error; List populates Services, DumpLogs populates Path. One Result type
// (rather than per-verb structs) threads more easily through generic transports
// at the cost of a slightly fatter struct.
type Result struct {
	Ok       bool                   `json:"ok"`
	Error    string                 `json:"error,omitempty"`
	Services []protocol.ServiceInfo `json:"services,omitempty"`
	Path     string                 `json:"path,omitempty"`
	// Lines is the captured-log buffer returned by Resync for a single
	// service. Empty for every other verb.
	Lines []string `json:"lines,omitempty"`
	// LinesByService is the multi-service variant returned when Resync
	// is called with Service=="". Keyed by service name.
	LinesByService map[string][]string `json:"lines_by_service,omitempty"`
}

// NotImplemented is the conventional result for verbs whose wire shape exists
// but whose server-side handler has not shipped. Callers see a clean error
// string instead of silent success.
func NotImplemented(verb string) Result {
	return Result{Ok: false, Error: "verb " + verb + " is not yet implemented on this server"}
}
