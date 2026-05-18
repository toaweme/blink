// Package protocol defines the canonical message types that flow from a
// running blink supervisor out to consumers - the local TUI, the plain UI,
// the headless log writer, and remote mirrors connected via a Transport.
//
// Everything that crosses a boundary (process, socket, network) is one of
// these. The supervisor emits StatusEvent / LogLine; mirrored clients receive
// ConfigSnapshot once and StatusEvent / LogLine continuously; control flows
// the other way as ControlCommand → ControlResult.
//
// Envelope is the wire frame. Each typed payload knows its Kind so a
// transport can demultiplex without reflection.
package protocol

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/toaweme/blink/core/config"
)

// Kind discriminates a message payload inside an Envelope.
type Kind string

const (
	KindStatus  Kind = "status"
	KindLog     Kind = "log"
	KindConfig  Kind = "config"
	KindControl Kind = "control"
	KindResult  Kind = "result"
)

// Envelope is the framed wire format. Payload is a JSON-encoded value whose
// concrete type is determined by Kind. At is the publisher's clock at emit
// time; consumers should treat it as advisory (clocks drift).
type Envelope struct {
	Kind    Kind            `json:"kind"`
	Payload json.RawMessage `json:"payload"`
	At      time.Time       `json:"at,omitempty"`
}

// StatusEvent reports a lifecycle transition for a service or one of its
// managed children (e.g. compose containers). Status strings match the
// supervisor's Status constants verbatim - keeping them stringly-typed at
// this layer lets new runtimes invent new states without a protocol bump.
//
// Child is empty for the service itself; non-empty for nested processes.
type StatusEvent struct {
	Service string    `json:"service"`
	Child   string    `json:"child,omitempty"`
	Status  string    `json:"status"`
	Err     string    `json:"err,omitempty"`
	At      time.Time `json:"at"`
}

// LogLine is a single line of captured output from a service or a managed
// child. Lines are pre-split on '\n' by the publisher; consumers do not
// need to re-split.
type LogLine struct {
	Service string    `json:"service"`
	Child   string    `json:"child,omitempty"`
	Line    string    `json:"line"`
	At      time.Time `json:"at,omitempty"`
}

// ConfigSnapshot is the host's effective config.Config at the moment of
// connection. Mirrored clients receive one on connect so that saved search
// presets, service names, and dependency edges are available locally
// without re-reading blink.yaml. The client view is read-only against
// this config; writes back to the host go through ControlCommand.
type ConfigSnapshot struct {
	Config config.Config `json:"config"`
}

// ServiceInfo is one row in a List-command result. Lives in protocol
// (rather than control) so streaming consumers that only want to look
// at status snapshots without dragging in the verb catalog can use it.
type ServiceInfo struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Pid    int    `json:"pid,omitempty"`
	Stdin  bool   `json:"stdin,omitempty"`
}

// Encode wraps a typed payload in an Envelope ready for the wire.
func Encode(kind Kind, payload any) (Envelope, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, fmt.Errorf("failed to encode %s payload: %w", kind, err)
	}
	return Envelope{Kind: kind, Payload: raw, At: time.Now()}, nil
}

// DecodeStatus unmarshals env.Payload into a StatusEvent. Errors if Kind
// doesn't match.
func DecodeStatus(env Envelope) (StatusEvent, error) {
	if env.Kind != KindStatus {
		return StatusEvent{}, fmt.Errorf("expected status envelope, got %q", env.Kind)
	}
	var ev StatusEvent
	if err := json.Unmarshal(env.Payload, &ev); err != nil {
		return StatusEvent{}, fmt.Errorf("failed to decode status payload: %w", err)
	}
	return ev, nil
}

// DecodeLog unmarshals env.Payload into a LogLine.
func DecodeLog(env Envelope) (LogLine, error) {
	if env.Kind != KindLog {
		return LogLine{}, fmt.Errorf("expected log envelope, got %q", env.Kind)
	}
	var ln LogLine
	if err := json.Unmarshal(env.Payload, &ln); err != nil {
		return LogLine{}, fmt.Errorf("failed to decode log payload: %w", err)
	}
	return ln, nil
}

// DecodeConfig unmarshals env.Payload into a ConfigSnapshot.
func DecodeConfig(env Envelope) (ConfigSnapshot, error) {
	if env.Kind != KindConfig {
		return ConfigSnapshot{}, fmt.Errorf("expected config envelope, got %q", env.Kind)
	}
	var c ConfigSnapshot
	if err := json.Unmarshal(env.Payload, &c); err != nil {
		return ConfigSnapshot{}, fmt.Errorf("failed to decode config payload: %w", err)
	}
	return c, nil
}
