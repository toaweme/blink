package config

import (
	"path/filepath"
	"testing"
)

func Test_Paths_Resolve(t *testing.T) {
	const root = "/project"
	// a var (not a Join literal) so the wantLog/wantBuild Joins below have a
	// non-literal base, keeping gocritic's filepathJoin check happy.
	custom := "/var/state"

	tests := []struct {
		name        string
		paths       Paths
		env         map[string]string
		wantControl string
		wantLog     string
		wantBuild   string
	}{
		{
			name:        "defaults derive under the control dir",
			wantControl: filepath.Join(root, ".blink"),
			wantLog:     filepath.Join(root, ".blink", "logs"),
			wantBuild:   filepath.Join(root, ".blink", "build"),
		},
		{
			name:        "custom control dir flows to log and build",
			paths:       Paths{ControlDir: custom},
			wantControl: custom,
			wantLog:     filepath.Join(custom, "logs"),
			wantBuild:   filepath.Join(custom, "build"),
		},
		{
			name:        "relative env override resolves against root",
			env:         map[string]string{"BLINK_BUILD_DIR": "out/bin"},
			wantControl: filepath.Join(root, ".blink"),
			wantLog:     filepath.Join(root, ".blink", "logs"),
			wantBuild:   filepath.Join(root, "out", "bin"),
		},
		{
			name:        "absolute env override is honored as-is",
			env:         map[string]string{"BLINK_LOG_DIR": "/tmp/blogs"},
			wantControl: filepath.Join(root, ".blink"),
			wantLog:     "/tmp/blogs",
			wantBuild:   filepath.Join(root, ".blink", "build"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// deterministic regardless of the host env: clear every path var,
			// then apply only what the case sets. Empty reads as "no override".
			for _, k := range []string{"BLINK_CONTROL_DIR", "BLINK_LOG_DIR", "BLINK_BUILD_DIR"} {
				t.Setenv(k, "")
			}
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			p := tt.paths
			p.Resolve(root)

			if p.ControlDir != tt.wantControl {
				t.Errorf("ControlDir = %q, want %q", p.ControlDir, tt.wantControl)
			}
			if p.LogDir != tt.wantLog {
				t.Errorf("LogDir = %q, want %q", p.LogDir, tt.wantLog)
			}
			if p.BuildDir != tt.wantBuild {
				t.Errorf("BuildDir = %q, want %q", p.BuildDir, tt.wantBuild)
			}
		})
	}
}
