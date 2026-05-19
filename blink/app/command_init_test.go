package app

import (
	"os"
	"path/filepath"
	"testing"
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
			if len(s.Ports) != 1 || s.Ports[0] != 8081 {
				t.Fatalf("go service ports = %v, want [8081]", s.Ports)
			}
		}
	}
	if !found {
		t.Fatalf("no go service detected in %v", services)
	}
}
