package control

import (
	"encoding/json"
	"fmt"

	"github.com/toaweme/blink/core/protocol"
)

// envelope is the wire shape for a control command, nested inside a
// protocol.Envelope's Payload (Kind = KindControl). Two-level framing keeps the
// streaming side symmetric with control while letting control carry a typed
// verb plus payload.
type envelope struct {
	ID      string          `json:"id,omitempty"`
	Verb    string          `json:"verb"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// resultEnvelope is the response analog. Kind = KindResult.
type resultEnvelope struct {
	ID     string `json:"id,omitempty"`
	Result Result `json:"result"`
}

// EncodeCommand wraps a typed Command as a protocol.Envelope for a Transport.
// id is an optional correlation token the matching result echoes.
func EncodeCommand(cmd Command, id string) (protocol.Envelope, error) {
	raw, err := json.Marshal(cmd)
	if err != nil {
		return protocol.Envelope{}, fmt.Errorf("failed to encode %s command: %w", cmd.Verb(), err)
	}
	body, err := json.Marshal(envelope{ID: id, Verb: cmd.Verb(), Payload: raw})
	if err != nil {
		return protocol.Envelope{}, fmt.Errorf("failed to wrap %s command: %w", cmd.Verb(), err)
	}
	return protocol.Envelope{Kind: protocol.KindControl, Payload: body}, nil
}

// DecodeCommand unmarshals a control envelope into the concrete Command for its
// verb, returning the command and its correlation ID. Unknown verbs surface as
// an explicit error so the server can reply with NotImplemented.
func DecodeCommand(env protocol.Envelope) (Command, string, error) {
	if env.Kind != protocol.KindControl {
		return nil, "", fmt.Errorf("expected control envelope, got %q", env.Kind)
	}
	var w envelope
	if err := json.Unmarshal(env.Payload, &w); err != nil {
		return nil, "", fmt.Errorf("failed to decode control envelope: %w", err)
	}
	cmd, err := decodeByVerb(w.Verb, w.Payload)
	if err != nil {
		return nil, w.ID, err
	}
	return cmd, w.ID, nil
}

// EncodeResult wraps a Result as a KindResult envelope echoing id.
func EncodeResult(id string, res Result) (protocol.Envelope, error) {
	body, err := json.Marshal(resultEnvelope{ID: id, Result: res})
	if err != nil {
		return protocol.Envelope{}, fmt.Errorf("failed to encode result: %w", err)
	}
	return protocol.Envelope{Kind: protocol.KindResult, Payload: body}, nil
}

// DecodeResult unmarshals a result envelope. Returns the Result and the
// correlation ID the command sent with EncodeCommand.
func DecodeResult(env protocol.Envelope) (Result, string, error) {
	if env.Kind != protocol.KindResult {
		return Result{}, "", fmt.Errorf("expected result envelope, got %q", env.Kind)
	}
	var w resultEnvelope
	if err := json.Unmarshal(env.Payload, &w); err != nil {
		return Result{}, "", fmt.Errorf("failed to decode result envelope: %w", err)
	}
	return w.Result, w.ID, nil
}

// decodeByVerb maps a wire verb to its concrete Command struct. Adding a verb
// means declaring the struct in commands.go and adding an arm here. A switch,
// not a registry, keeps the catalog visible in one file.
func decodeByVerb(verb string, payload json.RawMessage) (Command, error) {
	switch verb {
	case VerbList:
		var c List
		return c, unmarshalIfPresent(payload, &c)
	case VerbRestart:
		var c Restart
		return c, unmarshalIfPresent(payload, &c)
	case VerbSend:
		var c Send
		return c, unmarshalIfPresent(payload, &c)
	case VerbSignal:
		var c Signal
		return c, unmarshalIfPresent(payload, &c)
	case VerbDumpLogs:
		var c DumpLogs
		return c, unmarshalIfPresent(payload, &c)
	case VerbPause:
		var c Pause
		return c, unmarshalIfPresent(payload, &c)
	case VerbResume:
		var c Resume
		return c, unmarshalIfPresent(payload, &c)
	case VerbReloadConfig:
		var c ReloadConfig
		return c, unmarshalIfPresent(payload, &c)
	case VerbResync:
		var c Resync
		return c, unmarshalIfPresent(payload, &c)
	default:
		return nil, fmt.Errorf("unknown verb %q", verb)
	}
}

func unmarshalIfPresent(p json.RawMessage, into any) error {
	if len(p) == 0 || string(p) == "null" {
		return nil
	}
	if err := json.Unmarshal(p, into); err != nil {
		return fmt.Errorf("failed to decode payload: %w", err)
	}
	return nil
}
