package control

import (
	"context"
	"sync"
)

// Handler is the callback shape for one verb. Each one receives the
// concrete typed Command (already decoded) and the request context.
// Returning Result.Ok=false with Error set is the only failure mode;
// a Go error from Handler is not part of the contract.
type Handler func(ctx context.Context, cmd Command) Result

// Dispatcher routes typed Commands to per-verb handlers. Each handler
// is registered with a minimum required Role; Dispatch consults the
// peer's role (supplied at call time by session.Server) before
// invoking the handler. Below-minimum peers get a ForbiddenResult
// without the handler ever running.
//
// Built from scratch with NewDispatcher, populated with Register,
// queried with Dispatch. The supervisor (or its wiring code) is the
// canonical builder: it knows how to satisfy each verb.
type Dispatcher struct {
	mu       sync.RWMutex
	handlers map[string]registered
}

type registered struct {
	role Role
	fn   Handler
}

// NewDispatcher returns an empty dispatcher.
func NewDispatcher() *Dispatcher {
	return &Dispatcher{handlers: make(map[string]registered)}
}

// Register binds verb to handler with a minimum required role.
// Overwrites silently - the wiring code is allowed to replace stubs
// once a real implementation arrives.
func (d *Dispatcher) Register(verb string, role Role, h Handler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.handlers[verb] = registered{role: role, fn: h}
}

// Dispatch runs the handler registered for cmd.Verb() iff peerRole is
// at or above the handler's required role. Verbs with no handler get a
// NotImplemented result; verbs whose role check fails get a
// ForbiddenResult. Either way the caller always sees a concrete answer
// rather than a hang.
func (d *Dispatcher) Dispatch(ctx context.Context, cmd Command, peerRole Role) Result {
	d.mu.RLock()
	h, ok := d.handlers[cmd.Verb()]
	d.mu.RUnlock()
	if !ok {
		return NotImplemented(cmd.Verb())
	}
	if !peerRole.AtLeast(h.role) {
		return ForbiddenResult(cmd.Verb(), peerRole, h.role)
	}
	return h.fn(WithRole(ctx, peerRole), cmd)
}
