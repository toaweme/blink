package detect

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFiles materialises a map of relative path -> content under a fresh temp
// dir and returns the dir. Nested paths are created as needed.
func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	return dir
}

func names(d []Detected) []string {
	out := make([]string, 0, len(d))
	for _, x := range d {
		out = append(out, x.Service.Name)
	}
	return out
}

func find(d []Detected, name string) (Detected, bool) {
	for _, x := range d {
		if x.Service.Name == name {
			return x, true
		}
	}
	return Detected{}, false
}

func Test_Scan_MixedRepo(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"go.mod":             "module github.com/acme/widgets\n\ngo 1.22\n",
		"cmd/api/main.go":    "package main\n\nfunc main() {}\n",
		"cmd/worker/main.go": "package main\n\nfunc main() {}\n",
		"Procfile":           "release: ./migrate.sh\n",
	})

	cfg, detected, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if cfg.DirRoot != dir {
		t.Fatalf("DirRoot = %q, want %q", cfg.DirRoot, dir)
	}
	if len(cfg.Services) != len(detected) {
		t.Fatalf("cfg has %d services, detected %d", len(cfg.Services), len(detected))
	}

	// names must be unique across the whole scan.
	seen := map[string]bool{}
	for _, n := range names(detected) {
		if seen[n] {
			t.Fatalf("duplicate service name %q in %v", n, names(detected))
		}
		seen[n] = true
	}

	for _, want := range []string{"api", "worker", "release"} {
		if _, ok := find(detected, want); !ok {
			t.Fatalf("missing service %q in %v", want, names(detected))
		}
	}
}

func Test_Scan_AirSupersedesGo(t *testing.T) {
	// air is a hot-reloader for the same go binaries, so an .air.<name>.toml
	// and the go service for ./cmd/<name> describe the same process. the air
	// entry must win and the go duplicate must be dropped, not suffixed.
	dir := writeFiles(t, map[string]string{
		"go.mod":                  "module github.com/acme/awee\n\ngo 1.22\n",
		"cmd/v2/registry/main.go": "package main\nfunc main() {}\n",
		"cmd/v2/schema/main.go":   "package main\nfunc main() {}\n",
		".air.registry.toml":      "[build]\n  bin = \"./tmp/registry\"\n",
		".air.schema.toml":        "[build]\n  bin = \"./tmp/schema\"\n",
	})

	_, detected, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	for _, n := range names(detected) {
		if n == "registry-2" || n == "schema-2" {
			t.Fatalf("go duplicate was suffixed instead of dropped: %v", names(detected))
		}
	}
	for _, name := range []string{"registry", "schema"} {
		d, ok := find(detected, name)
		if !ok {
			t.Fatalf("missing service %q in %v", name, names(detected))
		}
		if d.Source != "air" {
			t.Fatalf("%q kept the %s source, want air to win", name, d.Source)
		}
	}
}

func Test_GoDetector_MainPackages(t *testing.T) {
	tests := []struct {
		name      string
		files     map[string]string
		wantNames []string
		wantPkgs  map[string]string // service name -> Go.Package
	}{
		{
			name: "single root main",
			files: map[string]string{
				"go.mod":  "module github.com/acme/tool\n",
				"main.go": "package main\nfunc main() {}\n",
			},
			wantNames: []string{"tool"},
			wantPkgs:  map[string]string{"tool": "."},
		},
		{
			name: "cmd subpackages",
			files: map[string]string{
				"go.mod":             "module github.com/acme/svc\n",
				"cmd/api/main.go":    "package main\nfunc main() {}\n",
				"cmd/worker/main.go": "package main\nfunc main() {}\n",
				"internal/lib/x.go":  "package lib\n",
			},
			wantNames: []string{"api", "worker"},
			wantPkgs:  map[string]string{"api": "./cmd/api", "worker": "./cmd/worker"},
		},
		{
			name: "no main falls back to root",
			files: map[string]string{
				"go.mod": "module github.com/acme/lib\n",
				"lib.go": "package lib\n",
			},
			wantNames: []string{"lib"},
			wantPkgs:  map[string]string{"lib": "."},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeFiles(t, tt.files)
			got, err := goDetector{}.Detect(dir)
			if err != nil {
				t.Fatalf("Detect: %v", err)
			}
			if len(got) != len(tt.wantNames) {
				t.Fatalf("got %v, want names %v", names(got), tt.wantNames)
			}
			for name, pkg := range tt.wantPkgs {
				d, ok := find(got, name)
				if !ok {
					t.Fatalf("missing %q in %v", name, names(got))
				}
				if d.Service.Go == nil || d.Service.Go.Package != pkg {
					t.Fatalf("%q package = %+v, want %q", name, d.Service.Go, pkg)
				}
			}
		})
	}
}

func Test_AirDetector_PerFileServices(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		".air.toml": `root = "."
[build]
  cmd = "go build -o ./tmp/main ."
  bin = "./tmp/main"
  args_bin = ["--flag"]
  include_ext = ["go", ".tmpl"]
  exclude_dir = ["tmp", "vendor", "node_modules", "ui/node_modules", "build"]
`,
		".air.registry.toml": "[build]\n  bin = \"./tmp/registry\"\n",
		".air.slack.toml":    "[build]\n  full_bin = \"./tmp/slack --verbose\"\n",
		".air.schema.toml":   "[build]\n  bin = \"./tmp/schema\"\n",
		".air.workflow.toml": "[build]\n  bin = \"./tmp/workflow\"\n",
	})

	got, err := airDetector{}.Detect(dir)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("got %d services %v, want 5", len(got), names(got))
	}

	// .air.toml carries a `go build` cmd, so it becomes a native go runtime:
	// package from the build, args from args_bin. No shell Commands.
	root, ok := find(got, filepath.Base(dir))
	if !ok {
		t.Fatalf("missing root air service in %v", names(got))
	}
	if root.Service.Runtime != "go" {
		t.Fatalf("root runtime = %q, want go", root.Service.Runtime)
	}
	if root.Service.Go == nil || root.Service.Go.Package != "." {
		t.Fatalf("root go package = %+v, want .", root.Service.Go)
	}
	if got, want := root.Service.Go.Args, []string{"--flag"}; !equalStrings(got, want) {
		t.Fatalf("root args = %v, want %v", got, want)
	}
	if root.Service.Commands.Run != nil || root.Service.Commands.Build != nil {
		t.Fatalf("go-runtime air service should not carry shell Commands: %+v", root.Service.Commands)
	}
	if got, want := root.Service.Fs.Extensions, []string{"go", "tmpl"}; !equalStrings(got, want) {
		t.Fatalf("root extensions = %v, want %v (dots stripped)", got, want)
	}
	// air's exclude_dir names become path globs so they match anywhere in the
	// tree; bare names would never match a full path. Entries blink already
	// excludes by default (node_modules, ui/node_modules, build) are dropped.
	if got, want := root.Service.Fs.Exclude, []string{"**/tmp/**", "**/vendor/**"}; !equalStrings(got, want) {
		t.Fatalf("root exclude = %v, want %v", got, want)
	}

	// the other configs declare only bin/full_bin (no go-build cmd), so they
	// stay faithful shell services running that binary.
	wantShell := map[string]string{
		"registry": "./tmp/registry",
		"slack":    "./tmp/slack --verbose",
		"schema":   "./tmp/schema",
		"workflow": "./tmp/workflow",
	}
	for name, run := range wantShell {
		d, ok := find(got, name)
		if !ok {
			t.Fatalf("missing air service %q in %v", name, names(got))
		}
		if d.Service.Runtime != "shell" {
			t.Fatalf("%q runtime = %q, want shell", name, d.Service.Runtime)
		}
		if d.Service.Commands.Run == nil || d.Service.Commands.Run.Command != run {
			t.Fatalf("%q run = %+v, want %q", name, d.Service.Commands.Run, run)
		}
		if !d.Service.Reload.Reload {
			t.Fatalf("%q should have reload enabled", name)
		}
	}
}

func Test_ParseGoBuild(t *testing.T) {
	tests := []struct {
		name    string
		cmd     string
		wantPkg string
		wantOK  bool
	}{
		{"root build", "go build -o ./build/app .", ".", true},
		{"sub package", "go build -o ./build/api ./cmd/api", "./cmd/api", true},
		{"cd into module", "cd cmd/v2/registry && go build -o ../../../build/registry .", "./cmd/v2/registry", true},
		{"clear then build", "clear && go build -o ./build/app .", ".", true},
		{"no explicit package", "go build -o ./build/app", ".", true},
		{"not go", "make build", "", false},
		{"npm", "npm run build", "", false},
		{"empty", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkg, ok := parseGoBuild(tt.cmd)
			if ok != tt.wantOK || pkg != tt.wantPkg {
				t.Fatalf("parseGoBuild(%q) = (%q, %v), want (%q, %v)", tt.cmd, pkg, ok, tt.wantPkg, tt.wantOK)
			}
		})
	}
}

func Test_AirDetector_MalformedIsError(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		".air.toml": "[build\n  this is not = valid toml",
	})
	_, err := airDetector{}.Detect(dir)
	if err == nil {
		t.Fatalf("expected error for malformed air toml")
	}
}

func Test_PythonDetector(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string
		wantRun string
	}{
		{
			name:    "django manage.py",
			files:   map[string]string{"manage.py": "", "requirements.txt": ""},
			wantRun: "python manage.py runserver",
		},
		{
			name:    "main.py entrypoint",
			files:   map[string]string{"main.py": "", "pyproject.toml": ""},
			wantRun: "python main.py",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeFiles(t, tt.files)
			got, err := pythonDetector{}.Detect(dir)
			if err != nil {
				t.Fatalf("Detect: %v", err)
			}
			if len(got) != 1 {
				t.Fatalf("got %d services, want 1", len(got))
			}
			if got[0].Service.Commands.Run.Command != tt.wantRun {
				t.Fatalf("run = %q, want %q", got[0].Service.Commands.Run.Command, tt.wantRun)
			}
		})
	}
}

func Test_RustDetector(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"Cargo.toml": "[package]\nname = \"mycrate\"\nversion = \"0.1.0\"\n",
	})
	got, err := rustDetector{}.Detect(dir)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(got) != 1 || got[0].Service.Name != "mycrate" {
		t.Fatalf("got %v, want [mycrate]", names(got))
	}
	if got[0].Service.Commands.Run.Command != "cargo run" {
		t.Fatalf("run = %q", got[0].Service.Commands.Run.Command)
	}
}

func Test_ProcfileDetector(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"Procfile": "# comment\nweb: bundle exec rails server\nworker: bundle exec sidekiq\n\nbad-line-without-colon\n",
	})
	got, err := procfileDetector{}.Detect(dir)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if !equalStrings(names(got), []string{"web", "worker"}) {
		t.Fatalf("got %v, want [web worker]", names(got))
	}
	web, _ := find(got, "web")
	if web.Service.Commands.Run.Command != "bundle exec rails server" {
		t.Fatalf("web run = %q", web.Service.Commands.Run.Command)
	}
}

func Test_Detectors_FindNothing(t *testing.T) {
	dir := t.TempDir()
	_, detected, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan empty dir: %v", err)
	}
	if len(detected) != 0 {
		t.Fatalf("expected nothing detected, got %v", names(detected))
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

func Test_Scan_DockerFirst(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"go.mod":             "module github.com/acme/widgets\n\ngo 1.22\n",
		"cmd/api/main.go":    "package main\n\nfunc main() {}\n",
		"docker-compose.yml": "services:\n  db:\n    image: postgres\n",
	})
	_, detected, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(detected) == 0 || detected[0].Service.Runtime != "docker" {
		t.Fatalf("first service = %v, want docker first", names(detected))
	}
}
