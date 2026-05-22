package control

import (
	"context"
	"sync"
)

// Handler is the callback for one verb, receiving the decoded Command and the
// request context. The only failure mode is Result.Ok=false with Error set; a
// Go error from Handler is not part of the contract.
type Handler func(ctx context.Context, cmd Command) Result

// Dispatcher routes typed Commands to per-verb handlers. Each handler is
// registered with a minimum required Role; Dispatch checks the peer's role
// before invoking the handler, returning a ForbiddenResult to below-minimum
// peers without running it.
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

// Register binds verb to handler with a minimum required role. Overwrites
// silently so wiring code can replace stubs with real implementations.
func (d *Dispatcher) Register(verb string, role Role, h Handler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.handlers[verb] = registered{role: role, fn: h}
}

// Dispatch runs the handler for cmd.Verb() when peerRole is at or above its
// required role. Verbs with no handler return NotImplemented; verbs failing the
// role check return ForbiddenResult. The caller always sees a concrete answer.
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
