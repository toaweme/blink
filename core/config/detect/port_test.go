package detect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/toaweme/blink/core/config"
)

func Test_EnvKeyForPort(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "api"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "api", ".env"), []byte("PORT=8080\nMETRICS_PORT=9100\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	svc := config.Service{Name: "api", Dir: "api"}

	if key, ok := EnvKeyForPort(root, svc, 8080); !ok || key != "PORT" {
		t.Fatalf("EnvKeyForPort(8080) = (%q, %v), want (PORT, true)", key, ok)
	}
	if key, ok := EnvKeyForPort(root, svc, 9100); !ok || key != "METRICS_PORT" {
		t.Fatalf("EnvKeyForPort(9100) = (%q, %v), want (METRICS_PORT, true)", key, ok)
	}
	if _, ok := EnvKeyForPort(root, svc, 1234); ok {
		t.Fatal("EnvKeyForPort(1234) = ok, want not found")
	}
}

func Test_SniffPorts(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string // relative path -> contents
		svc   config.Service
		want  []config.Port
	}{
		{
			name:  "bare PORT in service dir",
			files: map[string]string{"api/.env": "PORT=8080\n"},
			svc:   config.Service{Name: "api", Dir: "api"},
			want:  []config.Port{config.LiteralPort(8080)},
		},
		{
			name:  "addr with host and port, quoted",
			files: map[string]string{"web/.env": "HTTP_ADDR=\"0.0.0.0:3000\"\n"},
			svc:   config.Service{Name: "web", Dir: "web"},
			want:  []config.Port{config.LiteralPort(3000)},
		},
		{
			name:  "export prefix, comments, and url value",
			files: map[string]string{"svc/.env": "# comment\nexport SERVER_ADDR=http://localhost:9000/health\n"},
			svc:   config.Service{Name: "svc", Dir: "svc"},
			want:  []config.Port{config.LiteralPort(9000)},
		},
		{
			name:  "ignores non-port keys",
			files: map[string]string{"x/.env": "DATABASE_URL=postgres://u:p@host:5432/db\nNAME=api\n"},
			svc:   config.Service{Name: "x", Dir: "x"},
			want:  nil,
		},
		{
			name:  "dedupes across files",
			files: map[string]string{"y/.env": "PORT=8080\n", "y/.env.example": "PORT=8080\nMETRICS_PORT=9100\n"},
			svc:   config.Service{Name: "y", Dir: "y"},
			want:  []config.Port{config.LiteralPort(8080), config.LiteralPort(9100)},
		},
		{
			name:  "does not pull from the project root for a sub-dir service",
			files: map[string]string{".env": "PORT=4000\n"},
			svc:   config.Service{Name: "z", Dir: "z"},
			want:  nil,
		},
		{
			name:  "root-dir service reads the root .env",
			files: map[string]string{".env": "PORT=4000\n"},
			svc:   config.Service{Name: "root", Dir: ""},
			want:  []config.Port{config.LiteralPort(4000)},
		},
		{
			name:  "rejects out-of-range",
			files: map[string]string{"q/.env": "PORT=99999\n"},
			svc:   config.Service{Name: "q", Dir: "q"},
			want:  nil,
		},
	}

	portStrings := func(ports []config.Port) []string {
		out := make([]string, len(ports))
		for i, p := range ports {
			out[i] = p.String()
		}
		return out
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for rel, body := range tt.files {
				p := filepath.Join(root, rel)
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
					t.Fatalf("write: %v", err)
				}
			}
			got := SniffPorts(root, tt.svc)
			if len(got) != len(tt.want) {
				t.Fatalf("SniffPorts = %v, want %v", portStrings(got), portStrings(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("SniffPorts = %v, want %v", portStrings(got), portStrings(tt.want))
				}
			}
		})
	}
}
