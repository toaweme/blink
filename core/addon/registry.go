package addon

import (
	"context"
	"fmt"
	"sync"

	"github.com/toaweme/blink/core/config"
)

// Registry is the single place every addon the binary ships is wired in:
// runtimes and lifecycle hooks. The CLI's main builds one and registers
// explicitly: no init() side-effects, no blank imports.
//
// A nil *Registry is invalid for runtime lookups, but the hook dispatch helpers
// (DispatchHooks, HasHooks) tolerate it as no-ops so the supervisor can hold a
// registry that has no hooks.
type Registry struct {
	mu        sync.RWMutex
	runtimes  map[string]Runtime
	hooks     map[Phase][]ServiceHook
	hookNames map[string]struct{}
}

// NewRegistry returns an empty registry ready for the Add* calls.
func NewRegistry() *Registry {
	return &Registry{
		runtimes:  make(map[string]Runtime),
		hooks:     make(map[Phase][]ServiceHook),
		hookNames: make(map[string]struct{}),
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
