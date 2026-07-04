package node

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/toaweme/blink/core/config"
)

func boolPtr(b bool) *bool { return &b }

func Test_Node_Name(t *testing.T) {
	if got := (Runtime{}).Name(); got != "node" {
		t.Fatalf("Name() = %q, want %q", got, "node")
	}
}

func Test_Node_Prepare(t *testing.T) {
	tests := []struct {
		name        string
		node        *config.NodeConfig
		lockfile    string
		wantRun     string
		wantInstall string // "" means no Setup step is expected
	}{
		{
			name:        "defaults to npm run dev with a setup install",
			wantRun:     "npm run dev",
			wantInstall: "npm install",
		},
		{
			name:        "pnpm lockfile drives the manager",
			lockfile:    "pnpm-lock.yaml",
			wantRun:     "pnpm run dev",
			wantInstall: "pnpm install",
		},
		{
			name:        "bun text lockfile is recognized",
			lockfile:    "bun.lock",
			wantRun:     "bun run dev",
			wantInstall: "bun install",
		},
		{
			name:        "explicit script and manager win over detection",
			node:        &config.NodeConfig{Script: "build", PackageManager: "yarn"},
			lockfile:    "pnpm-lock.yaml",
			wantRun:     "yarn run build",
			wantInstall: "yarn install",
		},
		{
			name:        "install false suppresses the setup step",
			node:        &config.NodeConfig{Install: boolPtr(false)},
			wantRun:     "npm run dev",
			wantInstall: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.lockfile != "" {
				if err := os.WriteFile(filepath.Join(dir, tt.lockfile), nil, 0o600); err != nil {
					t.Fatalf("write lockfile: %v", err)
				}
			}

			svc := config.Service{Name: "web", Dir: ".", Node: tt.node}
			plan, err := Runtime{}.Prepare(config.Config{DirRoot: dir}, svc)
			if err != nil {
				t.Fatalf("Prepare: %v", err)
			}

			run := plan.Defaults.Commands.Run
			if run == nil {
				t.Fatal("Run command is nil")
			}
			if run.Command != tt.wantRun {
				t.Fatalf("Run.Command = %q, want %q", run.Command, tt.wantRun)
			}
			if !run.Service {
				t.Fatal("Run.Service = false, want true")
			}

			setup := plan.Defaults.Commands.Setup
			if tt.wantInstall == "" {
				if len(setup) != 0 {
					t.Fatalf("Setup = %v, want none", setup)
				}
				return
			}
			if len(setup) != 1 || setup[0].Command != tt.wantInstall {
				t.Fatalf("Setup = %v, want [%q]", setup, tt.wantInstall)
			}
			// install must never sit in the per-reload Run.Before chain.
			if len(run.Before) != 0 {
				t.Fatalf("Run.Before = %v, want none (install belongs in Setup)", run.Before)
			}
			if len(plan.SetupTriggers) == 0 {
				t.Fatal("SetupTriggers empty, want manifest/lockfiles")
			}
		})
	}
}

func Test_Node_PrepareExtensions(t *testing.T) {
	plan, err := Runtime{}.Prepare(config.Config{DirRoot: t.TempDir()}, config.Service{Dir: "."})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	has := func(ext string) bool {
		for _, e := range plan.Defaults.Fs.Extensions {
			if e == ext {
				return true
			}
		}
		return false
	}
	for _, want := range []string{"js", "jsx", "ts", "tsx", "json"} {
		if !has(want) {
			t.Fatalf("extensions %v missing %q", plan.Defaults.Fs.Extensions, want)
		}
	}
}

// Test_Node_PrepareReloadScoping asserts a self-reloading dev server (vite) is
// watched only for package.json + node_modules removal, so blink never restarts
// it over a source edit its HMR already handles, while a plain node server keeps
// the broad source watch.
func Test_Node_PrepareReloadScoping(t *testing.T) {
	tests := []struct {
		name        string
		devScript   string
		wantInclude []string // nil => none, keep broad Extensions
	}{
		{
			name:        "vite dev server scopes to package.json",
			devScript:   "vite dev --port 3000",
			wantInclude: []string{"package.json"},
		},
		{
			name:        "nodemon self-reloads too",
			devScript:   "nodemon server.js",
			wantInclude: []string{"package.json"},
		},
		{
			name:      "plain node server keeps broad source watch",
			devScript: "node server.js",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			pkg := `{"scripts":{"dev":"` + tt.devScript + `"}}`
			if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkg), 0o600); err != nil {
				t.Fatalf("write package.json: %v", err)
			}

			plan, err := Runtime{}.Prepare(config.Config{DirRoot: dir}, config.Service{Name: "web", Dir: "."})
			if err != nil {
				t.Fatalf("Prepare: %v", err)
			}
			fs := plan.Defaults.Fs

			if !equalStrings(fs.Include, tt.wantInclude) {
				t.Fatalf("Fs.Include = %v, want %v", fs.Include, tt.wantInclude)
			}
			if len(tt.wantInclude) > 0 {
				// strict include: source extensions must not be watched.
				if len(fs.Extensions) != 0 {
					t.Fatalf("Fs.Extensions = %v, want none for a self-reloading dev server", fs.Extensions)
				}
			} else if len(fs.Extensions) == 0 {
				t.Fatal("Fs.Extensions empty, want broad source watch for a plain node server")
			}
		})
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
