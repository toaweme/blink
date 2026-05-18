package control

import (
	"context"
	"encoding/json"
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/toaweme/blink/core/protocol"
)

// compile-time assertions that each command struct satisfies Command.
var (
	_ Command = List{}
	_ Command = Restart{}
	_ Command = Send{}
	_ Command = Signal{}
	_ Command = DumpLogs{}
	_ Command = Pause{}
	_ Command = Resume{}
	_ Command = ReloadConfig{}
	_ Command = Resync{}
)

func TestCommandRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		cmd  Command
	}{
		{"list", List{}},
		{"restart", Restart{Service: "api"}},
		{"send", Send{Service: "api", Data: "hello\n"}},
		{"signal", Signal{Service: "api", Signal: "TERM"}},
		{"dump-logs", DumpLogs{Service: "api", Path: "/tmp/x.log"}},
		{"pause", Pause{Service: "api"}},
		{"resume", Resume{Service: "api"}},
		{"reload-config", ReloadConfig{}},
		{"resync all", Resync{}},
		{"resync svc", Resync{Service: "api"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env, err := EncodeCommand(tc.cmd, "id-123")
			require.NoError(t, err)
			assert.Equal(t, protocol.KindControl, env.Kind)

			got, id, err := DecodeCommand(env)
			require.NoError(t, err)
			assert.Equal(t, "id-123", id)
			assert.Equal(t, tc.cmd.Verb(), got.Verb())
			assert.Equal(t, tc.cmd, got)
		})
	}
}

func TestDecodeCommandWrongKind(t *testing.T) {
	env := protocol.Envelope{Kind: protocol.KindStatus, Payload: []byte(`{}`)}
	_, _, err := DecodeCommand(env)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected control envelope")
}

func TestDecodeCommandUnknownVerb(t *testing.T) {
	body, err := json.Marshal(envelope{ID: "x", Verb: "bogus", Payload: []byte(`{}`)})
	require.NoError(t, err)
	env := protocol.Envelope{Kind: protocol.KindControl, Payload: body}
	_, id, err := DecodeCommand(env)
	require.Error(t, err)
	assert.Equal(t, "x", id)
	assert.Contains(t, err.Error(), "unknown verb")
}

func TestResultRoundTrip(t *testing.T) {
	res := Result{Ok: true, Path: "/tmp/api.log"}
	env, err := EncodeResult("rid-1", res)
	require.NoError(t, err)
	assert.Equal(t, protocol.KindResult, env.Kind)

	got, id, err := DecodeResult(env)
	require.NoError(t, err)
	assert.Equal(t, "rid-1", id)
	assert.Equal(t, res, got)
}

func TestResultRoundTrip_ResyncFields(t *testing.T) {
	res := Result{
		Ok:    true,
		Lines: []string{"line one", "line two"},
		LinesByService: map[string][]string{
			"api": {"hello", "world"},
			"db":  {"ready"},
		},
	}
	env, err := EncodeResult("rid-2", res)
	require.NoError(t, err)

	got, id, err := DecodeResult(env)
	require.NoError(t, err)
	assert.Equal(t, "rid-2", id)
	assert.Equal(t, res, got)
}

func TestDecodeResultWrongKind(t *testing.T) {
	env := protocol.Envelope{Kind: protocol.KindControl, Payload: []byte(`{}`)}
	_, _, err := DecodeResult(env)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected result envelope")
}

func TestDispatcherNotImplemented(t *testing.T) {
	d := NewDispatcher()
	res := d.Dispatch(context.Background(), List{}, RoleAdmin)
	assert.False(t, res.Ok)
	assert.Contains(t, res.Error, VerbList)
	assert.Contains(t, res.Error, "not yet implemented")
}

func TestDispatcherRoutesToHandler(t *testing.T) {
	d := NewDispatcher()
	var seen Command
	d.Register(VerbRestart, RoleOperator, func(_ context.Context, cmd Command) Result {
		seen = cmd
		return Result{Ok: true}
	})
	// unrelated verb stays unhandled.
	d.Register(VerbSend, RoleOperator, func(_ context.Context, _ Command) Result {
		return Result{Ok: false, Error: "should not be called"}
	})

	in := Restart{Service: "api"}
	res := d.Dispatch(context.Background(), in, RoleOperator)
	assert.True(t, res.Ok)
	assert.Equal(t, in, seen)
}

func TestDispatcherDeniesBelowRequiredRole(t *testing.T) {
	d := NewDispatcher()
	called := false
	d.Register(VerbRestart, RoleOperator, func(_ context.Context, _ Command) Result {
		called = true
		return Result{Ok: true}
	})
	res := d.Dispatch(context.Background(), Restart{Service: "api"}, RoleViewer)
	assert.False(t, called)
	assert.False(t, res.Ok)
	assert.Contains(t, res.Error, "requires role operator")
}

func TestDispatcherStampsRoleInContext(t *testing.T) {
	d := NewDispatcher()
	var got Role
	d.Register(VerbList, RoleViewer, func(ctx context.Context, _ Command) Result {
		got = RoleFromContext(ctx)
		return Result{Ok: true}
	})
	d.Dispatch(context.Background(), List{}, RoleAdmin)
	assert.Equal(t, RoleAdmin, got)
}

func TestNotImplementedHelper(t *testing.T) {
	r := NotImplemented("custom")
	assert.False(t, r.Ok)
	assert.Contains(t, r.Error, "custom")
}

func TestParseSignal(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		want   os.Signal
		errStr string
	}{
		{"int bare", "INT", syscall.SIGINT, ""},
		{"int sig prefix lower", "sigint", syscall.SIGINT, ""},
		{"term mixed case", "Term", syscall.SIGTERM, ""},
		{"kill with prefix", "SIGKILL", syscall.SIGKILL, ""},
		{"hup", "hup", syscall.SIGHUP, ""},
		{"usr1 with prefix", "SigUsr1", syscall.SIGUSR1, ""},
		{"usr2", "USR2", syscall.SIGUSR2, ""},
		{"quit with whitespace", "  quit  ", syscall.SIGQUIT, ""},
		{"empty", "", nil, "missing signal name"},
		{"sig only", "SIG", nil, "missing signal name"},
		{"unknown", "FOO", nil, "unsupported signal"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseSignal(tc.input)
			if tc.errStr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errStr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
