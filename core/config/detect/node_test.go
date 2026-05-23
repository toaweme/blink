package detect

import (
	"os"
	"path/filepath"
	"testing"
)

func Test_Node_Detect(t *testing.T) {
	tests := []struct {
		name      string
		pkgJSON   string
		lockfile  string
		wantName  string
		wantPM    string
		wantScrpt string
		wantLabel string
	}{
		{
			name:      "vite dev app with pnpm lock",
			pkgJSON:   `{"name":"web","scripts":{"dev":"vite"},"devDependencies":{"vite":"^5"}}`,
			lockfile:  "pnpm-lock.yaml",
			wantName:  "web",
			wantPM:    "pnpm",
			wantScrpt: "dev",
			wantLabel: "web (vite)",
		},
		{
			name:      "next app falls back to start script",
			pkgJSON:   `{"name":"site","scripts":{"start":"next start"},"dependencies":{"next":"^14"}}`,
			wantName:  "site",
			wantPM:    "npm",
			wantScrpt: "start",
			wantLabel: "site (next.js)",
		},
		{
			name:      "unnamed package uses dir name and plain node label",
			pkgJSON:   `{"scripts":{"dev":"node ."}}`,
			wantPM:    "npm",
			wantScrpt: "dev",
			wantLabel: "(node)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(tt.pkgJSON), 0o600); err != nil {
				t.Fatalf("write package.json: %v", err)
			}
			if tt.lockfile != "" {
				if err := os.WriteFile(filepath.Join(dir, tt.lockfile), nil, 0o600); err != nil {
					t.Fatalf("write lockfile: %v", err)
				}
			}

			got, err := nodeDetector{}.Detect(dir)
			if err != nil {
				t.Fatalf("Detect: %v", err)
			}
			if len(got) != 1 {
				t.Fatalf("got %d detected, want 1", len(got))
			}
			d := got[0]
			if d.Service.Runtime != "node" {
				t.Fatalf("Runtime = %q, want node", d.Service.Runtime)
			}
			if d.Service.Node == nil {
				t.Fatal("Service.Node is nil")
			}
			if d.Service.Node.PackageManager != tt.wantPM {
				t.Fatalf("PackageManager = %q, want %q", d.Service.Node.PackageManager, tt.wantPM)
			}
			if d.Service.Node.Script != tt.wantScrpt {
				t.Fatalf("Script = %q, want %q", d.Service.Node.Script, tt.wantScrpt)
			}
			if !d.Service.Reload.Reload {
				t.Fatal("Reload = false, want true")
			}
			wantName := tt.wantName
			if wantName == "" {
				wantName = filepath.Base(dir)
			}
			if d.Service.Name != wantName {
				t.Fatalf("Name = %q, want %q", d.Service.Name, wantName)
			}
			wantLabel := tt.wantLabel
			if tt.wantName == "" {
				wantLabel = filepath.Base(dir) + " (node)"
			}
			if d.Label != wantLabel {
				t.Fatalf("Label = %q, want %q", d.Label, wantLabel)
			}
		})
	}
}

func Test_Node_DetectNoPackageJSON(t *testing.T) {
	got, err := nodeDetector{}.Detect(t.TempDir())
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if got != nil {
		t.Fatalf("got %v, want nil for a dir with no package.json", got)
	}
}

func Test_Node_DetectMalformedPackageJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write package.json: %v", err)
	}
	if _, err := (nodeDetector{}).Detect(dir); err == nil {
		t.Fatal("Detect should error on malformed package.json")
	}
}
