package addon

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/toaweme/blink/core/config"
)

// fakeHook is an inline ServiceHook used to drive registry tests.
type fakeHook struct {
	name   string
	phases []Phase
	err    error
	// calls records every (phase) the hook was invoked at.
	calls []Phase
}

func (f *fakeHook) Name() string    { return f.name }
func (f *fakeHook) Phases() []Phase { return f.phases }
func (f *fakeHook) Run(_ context.Context, phase Phase, _ config.Config, _ config.Service) error {
	f.calls = append(f.calls, phase)
	return f.err
}

func Test_Registry_HasHooks_Empty(t *testing.T) {
	r := NewRegistry()
	require.NotNil(t, r)
	assert.False(t, r.HasHooks(PhaseBeforeBuild))
	assert.False(t, r.HasHooks(PhaseAfterStart))
}

func Test_Registry_AddHook_HasHooksReflectsPhases(t *testing.T) {
	r := NewRegistry()
	h := &fakeHook{name: "portkill", phases: []Phase{PhaseBeforeStart, PhaseAfterStop}}
	r.AddHook(h)

	tests := []struct {
		phase Phase
		want  bool
	}{
		{PhaseBeforeStart, true},
		{PhaseAfterStop, true},
		{PhaseBeforeBuild, false},
		{PhaseAfterBuild, false},
		{PhaseBeforeStop, false},
		{PhaseAfterStart, false},
	}
	for _, tc := range tests {
		t.Run(string(tc.phase), func(t *testing.T) {
			assert.Equal(t, tc.want, r.HasHooks(tc.phase))
		})
	}
}

func Test_Registry_DuplicateHookPanics(t *testing.T) {
	r := NewRegistry()
	r.AddHook(&fakeHook{name: "dup", phases: []Phase{PhaseBeforeStart}})
	assert.PanicsWithValue(t, `hook "dup" already registered`, func() {
		r.AddHook(&fakeHook{name: "dup", phases: []Phase{PhaseAfterStart}})
	})
}

func Test_Registry_DispatchHooks_OnlyDeclaredPhases(t *testing.T) {
	r := NewRegistry()
	a := &fakeHook{name: "a", phases: []Phase{PhaseBeforeStart}}
	b := &fakeHook{name: "b", phases: []Phase{PhaseAfterStart}}
	c := &fakeHook{name: "c", phases: []Phase{PhaseBeforeStart, PhaseAfterStart}}
	r.AddHook(a)
	r.AddHook(b)
	r.AddHook(c)

	ctx := context.Background()
	r.DispatchHooks(ctx, PhaseBeforeStart, config.Config{}, config.Service{}, nil)

	assert.Equal(t, []Phase{PhaseBeforeStart}, a.calls)
	assert.Nil(t, b.calls)
	assert.Equal(t, []Phase{PhaseBeforeStart}, c.calls)

	r.DispatchHooks(ctx, PhaseAfterStart, config.Config{}, config.Service{}, nil)
	assert.Equal(t, []Phase{PhaseBeforeStart}, a.calls)
	assert.Equal(t, []Phase{PhaseAfterStart}, b.calls)
	assert.Equal(t, []Phase{PhaseBeforeStart, PhaseAfterStart}, c.calls)
}

func Test_Registry_DispatchHooks_NilErrFn(t *testing.T) {
	r := NewRegistry()
	r.AddHook(&fakeHook{
		name:   "boom",
		phases: []Phase{PhaseBeforeStart},
		err:    errors.New("kaboom"),
	})
	// passing nil errFn must not panic even when a hook returns an error.
	assert.NotPanics(t, func() {
		r.DispatchHooks(context.Background(), PhaseBeforeStart, config.Config{}, config.Service{}, nil)
	})
}

func Test_Registry_DispatchHooks_NilRegistryNoOp(t *testing.T) {
	var r *Registry
	called := false
	assert.NotPanics(t, func() {
		r.DispatchHooks(context.Background(), PhaseBeforeStart, config.Config{}, config.Service{}, func(hook string, err error) {
			called = true
		})
	})
	assert.False(t, called)
	assert.False(t, r.HasHooks(PhaseBeforeStart))
}

func Test_Registry_DispatchHooks_ReportsErrors(t *testing.T) {
	r := NewRegistry()
	errA := errors.New("a failed")
	errC := errors.New("c failed")
	r.AddHook(&fakeHook{name: "a", phases: []Phase{PhaseBeforeStart}, err: errA})
	r.AddHook(&fakeHook{name: "b", phases: []Phase{PhaseBeforeStart}})
	r.AddHook(&fakeHook{name: "c", phases: []Phase{PhaseBeforeStart}, err: errC})

	type record struct {
		name string
		err  error
	}
	var got []record
	r.DispatchHooks(context.Background(), PhaseBeforeStart, config.Config{}, config.Service{}, func(hook string, err error) {
		got = append(got, record{name: hook, err: err})
	})

	require.Len(t, got, 2)
	assert.Equal(t, "a", got[0].name)
	assert.ErrorIs(t, got[0].err, errA)
	assert.Equal(t, "c", got[1].name)
	assert.ErrorIs(t, got[1].err, errC)
}
