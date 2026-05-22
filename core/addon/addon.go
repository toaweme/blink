// Package addon defines the extension surfaces (Discovery, Transport, Runtime)
// that let blink grow without special-casing each integration in the CLI.
//
// Three orthogonal capabilities live here:
//
//   - Discovery (discovery.go): how two endpoints find each other (relay URL plus session ID, WebRTC signaling, Tailscale hostname, unix-socket path).
//   - Transport (transport.go): the framed byte pipe once Discovery resolves an endpoint (unix-socket connection, WebRTC data channel, relay SSE/WS, TCP over a tailnet).
//   - Runtime (core/runtime): per-ecosystem service backends (shell, go, docker).
//
// An addon may implement any combination. Capabilities are not looked up by
// addon name: Runtimes are picked by `runtime:` in blink.yaml, while
// Discoveries and Transports are chosen per-session by the orchestrator. This
// package hosts only the interface contracts, not a central registry.
package addon
