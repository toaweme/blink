package addon

import "context"

// Discovery resolves an opaque target string into a Transport ready for
// Send/Recv. It is the "how do two endpoints meet" half of remote access, kept
// separate from Transport because the two are independent: a relay addon's
// discovery is "here is the URL" while its transport is the SSE/WS pipe, and
// the same Transport could be reached via a different Discovery.
//
// The target string's shape is discovery-specific. Examples:
//
//   - unix-socket: filesystem path
//   - relay:      "<relay-base-url>/<session-id>"
//   - webrtc:     a session descriptor exchanged via signaling
//   - tailscale:  a tailnet hostname (or hostname + port)
//
// The orchestrator that owns the remote-access flow picks the Discovery and
// feeds it the target.
type Discovery interface {
	// Name identifies the discovery for diagnostics and config.
	Name() string

	// Resolve performs the exchange needed to produce a usable Transport. For
	// local schemes this is a straight dial; for WebRTC it includes the SDP
	// exchange via the signaling endpoint.
	Resolve(ctx context.Context, target string, auth Auth) (Transport, error)
}

// Auth is the per-call credential a remote Discovery or Listener needs. Local
// transports ignore it.
type Auth struct {
	// Token is the bearer credential (saved auth token or $BLINK_RELAY_TOKEN).
	Token string
	// Role is "client" (mirror viewer) or "host" (supervisor publishing into a
	// session). Empty defaults to "client".
	Role string
}

const (
	// RoleClient is the mirror-viewer side of a session.
	RoleClient = "client"
	// RoleHost is the supervisor-publishing side of a session.
	RoleHost = "host"
)
