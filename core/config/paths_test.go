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

func Test_Paths_ConfigHome(t *testing.T) {
	const home = "/home/user"

	tests := []struct {
		name       string
		configHome string
		env        string
		want       string
	}{
		{
			name: "default is .blink under home",
			want: filepath.Join(home, ".blink"),
		},
		{
			name:       "absolute config value honored as-is",
			configHome: "/etc/blink",
			want:       "/etc/blink",
		},
		{
			// a relative override is user-scoped, so it resolves against $HOME
			// (not the process cwd or the project dir_root), staying consistent
			// with ConfigHome's default.
			name:       "relative config value resolves against home",
			configHome: "cfg/blink",
			want:       filepath.Join(home, "cfg", "blink"),
		},
		{
			name: "relative env override resolves against home",
			env:  "envcfg",
			want: filepath.Join(home, "envcfg"),
		},
		{
			name: "absolute env override honored as-is",
			env:  "/srv/blink",
			want: "/srv/blink",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// pin $HOME so os.UserHomeDir is deterministic, and clear the config
			// home override unless the case sets it.
			t.Setenv("HOME", home)
			t.Setenv("BLINK_CONFIG_HOME", tt.env)

			p := Paths{ConfigHome: tt.configHome}
			p.Resolve("/project")

			if p.ConfigHome != tt.want {
				t.Errorf("ConfigHome = %q, want %q", p.ConfigHome, tt.want)
			}
		})
	}
}
