package protocol

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/toaweme/blink/core/config"
)

func Test_EncodeDecode_StatusRoundTrip(t *testing.T) {
	in := StatusEvent{
		Service: "api",
		Child:   "db",
		Status:  "running",
		Err:     "",
		At:      time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}
	env, err := Encode(KindStatus, in)
	require.NoError(t, err)
	assert.Equal(t, KindStatus, env.Kind)
	assert.False(t, env.At.IsZero())

	out, err := DecodeStatus(env)
	require.NoError(t, err)
	assert.Equal(t, in, out)
}

func Test_EncodeDecode_LogRoundTrip(t *testing.T) {
	in := LogLine{
		Service: "api",
		Child:   "",
		Line:    "hello world",
		At:      time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}
	env, err := Encode(KindLog, in)
	require.NoError(t, err)
	assert.Equal(t, KindLog, env.Kind)

	out, err := DecodeLog(env)
	require.NoError(t, err)
	assert.Equal(t, in, out)
}

func Test_EncodeDecode_ConfigRoundTrip(t *testing.T) {
	in := ConfigSnapshot{
		Config: config.Config{
			UI:      "plain",
			DirRoot: "/tmp/project",
			Services: []config.Service{
				{Name: "api"},
			},
		},
	}
	env, err := Encode(KindConfig, in)
	require.NoError(t, err)
	assert.Equal(t, KindConfig, env.Kind)

	out, err := DecodeConfig(env)
	require.NoError(t, err)
	assert.Equal(t, in.Config.UI, out.Config.UI)
	assert.Equal(t, in.Config.DirRoot, out.Config.DirRoot)
	require.Len(t, out.Config.Services, 1)
	assert.Equal(t, "api", out.Config.Services[0].Name)
}

func Test_Decode_RejectsWrongKind(t *testing.T) {
	// build a log envelope and try to decode it as status/config.
	logEnv, err := Encode(KindLog, LogLine{Service: "x", Line: "y"})
	require.NoError(t, err)

	_, err = DecodeStatus(logEnv)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected status envelope")

	_, err = DecodeConfig(logEnv)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected config envelope")

	statusEnv, err := Encode(KindStatus, StatusEvent{Service: "x", Status: "running"})
	require.NoError(t, err)
	_, err = DecodeLog(statusEnv)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected log envelope")
}

func Test_Encode_PreservesPayloadBytes(t *testing.T) {
	in := StatusEvent{Service: "api", Status: "running", At: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
	env, err := Encode(KindStatus, in)
	require.NoError(t, err)

	// the envelope payload must be the exact JSON marshaling of the input.
	want, err := json.Marshal(in)
	require.NoError(t, err)
	assert.JSONEq(t, string(want), string(env.Payload))
}

func Test_Payload_JSONSerialization(t *testing.T) {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	tests := []struct {
		name    string
		value   any
		decode  func([]byte) (any, error)
		wantKey string
	}{
		{
			name:  "StatusEvent",
			value: StatusEvent{Service: "api", Child: "db", Status: "running", Err: "boom", At: ts},
			decode: func(b []byte) (any, error) {
				var v StatusEvent
				err := json.Unmarshal(b, &v)
				return v, err
			},
			wantKey: `"service":"api"`,
		},
		{
			name:  "LogLine",
			value: LogLine{Service: "api", Line: "tick", At: ts},
			decode: func(b []byte) (any, error) {
				var v LogLine
				err := json.Unmarshal(b, &v)
				return v, err
			},
			wantKey: `"line":"tick"`,
		},
		{
			name:  "ConfigSnapshot",
			value: ConfigSnapshot{Config: config.Config{UI: "plain"}},
			decode: func(b []byte) (any, error) {
				var v ConfigSnapshot
				err := json.Unmarshal(b, &v)
				return v, err
			},
			wantKey: `"config"`,
		},
		{
			name:  "ServiceInfo",
			value: ServiceInfo{Name: "api", Status: "running", Pid: 1234, Stdin: true},
			decode: func(b []byte) (any, error) {
				var v ServiceInfo
				err := json.Unmarshal(b, &v)
				return v, err
			},
			wantKey: `"name":"api"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(tc.value)
			require.NoError(t, err)
			assert.Contains(t, string(raw), tc.wantKey)

			got, err := tc.decode(raw)
			require.NoError(t, err)
			assert.Equal(t, tc.value, got)
		})
	}
}
