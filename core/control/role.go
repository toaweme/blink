package control

import "context"

// Role is the privilege level granted to a connected peer. Roles are
// ordered: viewer < operator < admin. A handler's minimum required
// role is set at registration; the dispatcher denies any peer whose
// role is below that.
//
// Roles are decided by the transport at accept time, not by the
// command sender. Local unix-socket peers are admins (the filesystem
// permissions are the gate); relay peers default to viewer unless the
// session owner explicitly promotes them.
type Role string

const (
	// RoleViewer is the safe default. Sees ConfigSnapshot, streamed
	// StatusEvent + LogLine, and can call read-only verbs (List,
	// DumpLogs to a relative path under DirRoot). Cannot restart, send
	// stdin, signal, or touch absolute paths.
	RoleViewer Role = "viewer"

	// RoleOperator is the "trusted teammate" role. Viewer + can mutate
	// runtime state: Restart, Stdin (Send), Signal. Cannot break out of
	// DirRoot or touch the config.
	RoleOperator Role = "operator"

	// RoleAdmin is full access. Operator + DumpLogs to absolute paths,
	// Pause, Resume, ReloadConfig. The local unix-socket client is
	// always admin.
	RoleAdmin Role = "admin"
)

// rank turns Role into an int so comparisons are unambiguous. Higher
// rank = more privilege. Unknown roles rank as -1 so they fail every
// check.
func rank(r Role) int {
	switch r {
	case RoleViewer:
		return 1
	case RoleOperator:
		return 2
	case RoleAdmin:
		return 3
	default:
		return -1
	}
}

// AtLeast reports whether r is at or above required.
func (r Role) AtLeast(required Role) bool { return rank(r) >= rank(required) }

// ForbiddenResult is the response sent when a peer's role is below a
// verb's minimum. Distinct from NotImplemented so clients can show a
// clear "ask the host owner to promote you" hint instead of "this
// build doesn't ship that verb".
func ForbiddenResult(verb string, role, required Role) Result {
	return Result{
		Ok:    false,
		Error: "verb " + verb + " requires role " + string(required) + "; peer has " + string(role),
	}
}

// roleCtxKey is the private ctx key for the peer's role. Dispatcher
// stamps it; handlers that need to gate sub-features (e.g. dump-logs
// absolute paths) read it via RoleFromContext.
type roleCtxKey struct{}

// WithRole returns a child context carrying r. Used by the dispatcher
// to stamp the peer's role before invoking a handler.
func WithRole(ctx context.Context, r Role) context.Context {
	return context.WithValue(ctx, roleCtxKey{}, r)
}

// RoleFromContext extracts the peer's role from ctx, defaulting to
// RoleViewer when the ctx is unstamped (e.g. unit tests). Handlers
// should treat the default as "least privilege" - if a sub-feature
// requires admin, deny it on viewer.
func RoleFromContext(ctx context.Context) Role {
	if r, ok := ctx.Value(roleCtxKey{}).(Role); ok {
		return r
	}
	return RoleViewer
}
