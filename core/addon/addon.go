// Package addon defines the extension surfaces that let blink grow without
// special-casing each new integration in the CLI.
//
// Three orthogonal capabilities live here:
//
//   - Discovery (this file's neighbor discovery.go) - how two endpoints find
//     each other. Examples: a relay URL plus session ID, a WebRTC signaling
//     exchange, a Tailscale MagicDNS hostname, a Cloudflare tunnel URL, a
//     local unix-socket path.
//
//   - Transport (transport.go) - the framed byte pipe once Discovery has
//     resolved an endpoint. Examples: a unix-socket connection, a WebRTC
//     data channel, an HTTPS+SSE/WS connection through a relay, plain TCP
//     over a Tailscale tailnet.
//
//   - Runtime (lives in core/runtime) - per-ecosystem service
//     backends (shell, go, docker, future node/python/etc).
//
// An addon may implement any combination. The docker addon ships a Runtime
// today and could ship a log-source Transport later. The webrtc addon ships
// both a Discovery and a Transport. The relay addon's Discovery is "here is
// the URL" and its Transport is the SSE/WS pipe.
//
// Capabilities are NOT looked up by addon name. Each has its own selector:
// Runtimes are picked by `runtime:` in blink.yaml; Discoveries and
// Transports are chosen per-session by the orchestrator that owns the
// remote-access flow. So this package intentionally does not provide a
// central addon registry - it only hosts the interface contracts.
package addon
