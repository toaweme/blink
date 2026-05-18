package addon

import "context"

// Discovery resolves an opaque target string into a Transport ready for
// Send/Recv. It is the "how do two endpoints meet" half of remote access,
// kept separate from Transport because the two are independent: a WebRTC
// addon ships its own discovery (signaling exchange) AND its own
// transport (data channel), but a relay addon's discovery is "here's the
// URL" while its transport is the SSE/WS pipe - the same Transport could
// in principle be reached via a different Discovery.
//
// The target string's shape is discovery-specific. Examples:
//
//   - unix-socket: filesystem path
//   - relay:      "<relay-base-url>/<session-id>"
//   - webrtc:     a session descriptor exchanged via signaling
//   - tailscale:  a tailnet hostname (or hostname + port)
//
// The orchestrator that owns the remote-access flow picks the Discovery
// (typically from blink.yaml or a CLI flag) and feeds it the target.
type Discovery interface {
	// Name identifies the discovery for diagnostics and config.
	Name() string

	// Resolve performs whatever exchange is necessary to produce a usable
	// Transport. For purely local schemes (a unix socket path) this is a
	// straight dial; for WebRTC it includes the full SDP exchange via the
	// signaling endpoint.
	Resolve(ctx context.Context, target string, auth Auth) (Transport, error)
}

// Auth is the per-call credential a remote Discovery or Listener needs.
// Local transports (the unix control socket) ignore it.
type Auth struct {
	// Token is the bearer credential (saved auth token or $BLINK_RELAY_TOKEN).
	Token string
	// Role is "client" (mirror viewer) or "host" (the supervisor publishing
	// into a session). Empty defaults to "client".
	Role string
}

const (
	// RoleClient is the mirror-viewer side of a session.
	RoleClient = "client"
	// RoleHost is the supervisor-publishing side of a session.
	RoleHost = "host"
)
