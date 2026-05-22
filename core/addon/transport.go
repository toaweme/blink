package addon

import (
	"context"
	"io"

	"github.com/toaweme/blink/core/protocol"
)

// Transport is a framed bidirectional pipe of protocol.Envelopes, one logical
// connection between two endpoints. Framing, buffering, reconnection, and
// authentication are concerns of the concrete implementation.
//
// Implementations:
//   - unix-socket: ND-JSON envelopes over a local Unix domain socket.
//   - relay: HTTPS POST client to server, SSE server to client.
//   - webrtc: ordered+reliable data channel carrying length-prefixed envelopes.
//   - tailscale / cloudflare: TCP over the underlying tunnel.
//
// Callers may Send and Recv concurrently from different goroutines, but a
// single Send or a single Recv is not safe to call concurrently with itself.
// Close terminates both directions and unblocks any pending Recv with io.EOF.
type Transport interface {
	// Send delivers one envelope to the peer, blocking until it is queued on
	// the wire or ctx is canceled. Errors if the transport is closed or the
	// peer dropped.
	Send(ctx context.Context, env protocol.Envelope) error

	// Recv blocks for the next envelope from the peer. Returns io.EOF on a
	// clean peer close. Use ctx to abort a blocked Recv.
	Recv(ctx context.Context) (protocol.Envelope, error)

	// Close shuts the transport down. Idempotent. Pending Send/Recv callers
	// receive io.EOF, or context.Canceled if ctx fires first.
	Close() error
}

// Listener is the server-side counterpart of a Transport. An instance that
// accepts inbound connections (the local control socket, a relay endpoint, a
// WebRTC answerer) wraps its listening surface as a Listener.
type Listener interface {
	// Accept blocks for the next inbound Transport. Returns io.EOF when the
	// listener is closed.
	Accept(ctx context.Context) (Transport, error)

	// Close stops accepting. Idempotent. In-flight Accept calls unblock with
	// io.EOF.
	Close() error
}

// ListenSpec describes the session a ListenerFactory accepts peers for.
// URL/SessionID/Token identify the relay-mediated session; local listeners
// are built directly and ignore it.
type ListenSpec struct {
	URL       string
	SessionID string
	Token     string
}

// ListenerFactory builds a host-side Listener for a remote transport. The host
// registers one per scheme (relay, webrtc); the control surface looks it up by
// Name() to open a session listener without hardcoding the concrete addon.
type ListenerFactory interface {
	// Name identifies the transport ("relay", "webrtc").
	Name() string
	// Listen opens a Listener accepting peers for the given session.
	Listen(ctx context.Context, spec ListenSpec) (Listener, error)
}

// ErrClosed is returned by Send/Recv/Accept on a closed transport or listener.
// Wraps io.EOF so callers can errors.Is(err, io.EOF).
var ErrClosed = io.EOF
