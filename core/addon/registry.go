package addon

import (
	"context"
	"fmt"
	"sync"

	"github.com/toaweme/blink/core/config"
)

// Registry is the single place every addon the binary ships is wired in:
// runtimes, lifecycle hooks, and the client/host halves of remote
// transports. The CLI's main builds one and registers explicitly - no
// init() side-effects, no blank imports. blink-mini registers only
// runtimes + hooks, so the transport addons stay out of its import graph.
//
// A nil *Registry is not valid for runtime/transport lookups, but the hook
// dispatch helpers tolerate it (DispatchHooks / HasHooks are no-ops) so the
// supervisor can hold a registry that simply has no hooks.
type Registry struct {
	mu          sync.RWMutex
	runtimes    map[string]Runtime
	hooks       map[Phase][]ServiceHook
	hookNames   map[string]struct{}
	discoveries map[string]Discovery
	listeners   map[string]ListenerFactory
}

// NewRegistry returns an empty registry ready for the Add* calls.
func NewRegistry() *Registry {
	return &Registry{
		runtimes:    make(map[string]Runtime),
		hooks:       make(map[Phase][]ServiceHook),
		hookNames:   make(map[string]struct{}),
		discoveries: make(map[string]Discovery),
		listeners:   make(map[string]ListenerFactory),
	}
}

// AddRuntime registers one or more runtimes under their declared Name().
// Panics on a duplicate name so init-time wiring failures are loud.
func (r *Registry) AddRuntime(rts ...Runtime) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rt := range rts {
		name := rt.Name()
		if _, dup := r.runtimes[name]; dup {
			panic(fmt.Sprintf("runtime %q already registered", name))
		}
		r.runtimes[name] = rt
	}
}

// Runtime returns the runtime for name. The empty string maps to "shell".
func (r *Registry) Runtime(name string) (Runtime, error) {
	if name == "" {
		name = "shell"
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	rt, ok := r.runtimes[name]
	if !ok {
		return nil, fmt.Errorf("unknown runtime %q", name)
	}
	return rt, nil
}

// AddHook registers cross-cutting lifecycle hooks. Panics on duplicate name.
func (r *Registry) AddHook(hooks ...ServiceHook) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, h := range hooks {
		name := h.Name()
		if _, dup := r.hookNames[name]; dup {
			panic(fmt.Sprintf("hook %q already registered", name))
		}
		r.hookNames[name] = struct{}{}
		for _, p := range h.Phases() {
			r.hooks[p] = append(r.hooks[p], h)
		}
	}
}

// HasHooks reports whether any hook is registered for phase. The supervisor
// uses it to short-circuit phases nobody cares about. A nil registry has no
// hooks.
func (r *Registry) HasHooks(phase Phase) bool {
	if r == nil {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.hooks[phase]) > 0
}

// DispatchHooks runs every hook registered for phase. errFn, if provided,
// receives any per-hook error so the caller can log it with context. A nil
// registry is a no-op.
func (r *Registry) DispatchHooks(ctx context.Context, phase Phase, cfg config.Config, svc config.Service, errFn func(hook string, err error)) {
	if r == nil {
		return
	}
	r.mu.RLock()
	hooks := append([]ServiceHook(nil), r.hooks[phase]...)
	r.mu.RUnlock()
	for _, h := range hooks {
		if err := h.Run(ctx, phase, cfg, svc); err != nil && errFn != nil {
			errFn(h.Name(), err)
		}
	}
}

// AddTransportDiscovery registers client-side transport discoveries under
// their Name(). Panics on duplicate.
func (r *Registry) AddTransportDiscovery(ds ...Discovery) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, d := range ds {
		name := d.Name()
		if _, dup := r.discoveries[name]; dup {
			panic(fmt.Sprintf("transport discovery %q already registered", name))
		}
		r.discoveries[name] = d
	}
}

// Discovery returns the discovery registered under name.
func (r *Registry) Discovery(name string) (Discovery, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.discoveries[name]
	return d, ok
}

// AddTransportListener registers host-side listener factories under their
// Name(). Panics on duplicate.
func (r *Registry) AddTransportListener(ls ...ListenerFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, l := range ls {
		name := l.Name()
		if _, dup := r.listeners[name]; dup {
			panic(fmt.Sprintf("transport listener %q already registered", name))
		}
		r.listeners[name] = l
	}
}

// Listener returns the listener factory registered under name.
func (r *Registry) Listener(name string) (ListenerFactory, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	l, ok := r.listeners[name]
	return l, ok
}
