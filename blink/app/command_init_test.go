package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/toaweme/blink/core/config"
)

// Test_ScanServices_DetectsAndSniffsPorts covers the init entry path: a Go
// module with a main package is detected, and the port written in a sibling
// .env is attached to the service so the picker shows it up front.
func Test_ScanServices_DetectsAndSniffsPorts(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	write("go.mod", "module demo\n\ngo 1.25\n")
	write("cmd/api/main.go", "package main\n\nfunc main() {}\n")
	write(".env", "PORT=8081\n")

	services, err := scanServices(root)
	if err != nil {
		t.Fatalf("scanServices: %v", err)
	}
	if len(services) == 0 {
		t.Fatal("expected at least one detected service")
	}

	found := false
	for _, s := range services {
		if s.Runtime == "go" {
			found = true
			if len(s.Ports) != 1 || s.Ports[0] != config.LiteralPort(8081) {
				t.Fatalf("go service ports = %v, want [8081]", s.Ports)
			}
		}
	}
	if !found {
		t.Fatalf("no go service detected in %v", services)
	}
}

func Test_TrimWriteDefaults_Docker(t *testing.T) {
	root := t.TempDir()
	compose := "services:\n  db:\n    image: postgres\n  redis:\n    image: redis\n  api:\n    image: api\n"
	if err := os.WriteFile(filepath.Join(root, "docker-compose.yml"), []byte(compose), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	tests := []struct {
		name string
		in   []string
		want []string // nil => omitted
	}{
		{name: "full set is omitted", in: []string{"db", "redis", "api"}, want: nil},
		{name: "full set any order is omitted", in: []string{"api", "db", "redis"}, want: nil},
		{name: "real subset is kept", in: []string{"db", "redis"}, want: []string{"db", "redis"}},
		{name: "empty stays empty", in: nil, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.Config{Services: []config.Service{
				{Name: "docker", Runtime: "docker", Docker: &config.DockerConfig{Services: tt.in}},
			}}
			trimWriteDefaults(root, &cfg)
			docker := cfg.Services[0].Docker
			if tt.want == nil {
				// an all-default block is dropped entirely (no `docker: {}`).
				if docker != nil {
					t.Fatalf("Docker = %+v, want nil", docker)
				}
				return
			}
			got := docker.Services
			if len(got) != len(tt.want) {
				t.Fatalf("Services = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("Services = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func Test_TrimWriteDefaults_PreservesNonDefaultDocker(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "docker-compose.yml"), []byte("services:\n  db:\n    image: postgres\n"), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	// the full Services set is trimmed, but StopOnExit keeps the block alive.
	cfg := config.Config{Services: []config.Service{
		{Name: "docker", Runtime: "docker", Docker: &config.DockerConfig{Services: []string{"db"}, StopOnExit: true}},
	}}
	trimWriteDefaults(root, &cfg)
	d := cfg.Services[0].Docker
	if d == nil {
		t.Fatal("Docker dropped despite StopOnExit set")
	}
	if len(d.Services) != 0 {
		t.Fatalf("Services = %v, want trimmed to nil", d.Services)
	}
	if !d.StopOnExit {
		t.Fatal("StopOnExit lost")
	}
}

// Test_ScanServicesAt_RebasesDir covers the picker's add-from-path action: a Go
// service detected in a sibling directory is rebased so its Dir is relative to
// the project root, letting one blink.yaml supervise repos outside its tree.
func Test_ScanServicesAt_RebasesDir(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "app")
	ui := filepath.Join(root, "ui")
	for _, d := range []string{project, filepath.Join(ui, "cmd", "web")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	write := func(p, body string) {
		t.Helper()
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}
	write(filepath.Join(ui, "go.mod"), "module ui\n\ngo 1.25\n")
	write(filepath.Join(ui, "cmd", "web", "main.go"), "package main\n\nfunc main() {}\n")

	tests := []struct {
		name    string
		target  string // as the user would type it
		wantDir string
	}{
		{"relative sibling", "../ui", "../ui"},
		{"absolute path", ui, "../ui"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			services, err := scanServicesAt(project, tt.target)
			if err != nil {
				t.Fatalf("scanServicesAt: %v", err)
			}
			var web *config.Service
			for i := range services {
				if services[i].Runtime == "go" {
					web = &services[i]
				}
			}
			if web == nil {
				t.Fatalf("no go service detected in %v", services)
			}
			if web.Dir != tt.wantDir {
				t.Fatalf("Dir = %q, want %q", web.Dir, tt.wantDir)
			}
			// the go package stays relative to the (rebased) service dir.
			if web.Go == nil || web.Go.Package != "./cmd/web" {
				t.Fatalf("Go.Package = %v, want ./cmd/web", web.Go)
			}
		})
	}
}

func Test_ScanServicesAt_Errors(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "afile")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	tests := []struct {
		name   string
		target string
	}{
		{"missing directory", "../does-not-exist"},
		{"target is a file", "afile"},
		{"empty target", "   "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := scanServicesAt(root, tt.target); err == nil {
				t.Fatalf("scanServicesAt(%q) expected error", tt.target)
			}
		})
	}
}
