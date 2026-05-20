package detect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/toaweme/blink/core/config"
)

func Test_SniffPorts(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string // relative path -> contents
		svc   config.Service
		want  []int
	}{
		{
			name:  "bare PORT in service dir",
			files: map[string]string{"api/.env": "PORT=8080\n"},
			svc:   config.Service{Name: "api", Dir: "api"},
			want:  []int{8080},
		},
		{
			name:  "addr with host and port, quoted",
			files: map[string]string{"web/.env": "HTTP_ADDR=\"0.0.0.0:3000\"\n"},
			svc:   config.Service{Name: "web", Dir: "web"},
			want:  []int{3000},
		},
		{
			name: "export prefix, comments, and url value",
			files: map[string]string{"svc/.env": "# comment\nexport SERVER_ADDR=http://localhost:9000/health\n"},
			svc:  config.Service{Name: "svc", Dir: "svc"},
			want: []int{9000},
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
			want:  []int{8080, 9100},
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
			want:  []int{4000},
		},
		{
			name:  "rejects out-of-range",
			files: map[string]string{"q/.env": "PORT=99999\n"},
			svc:   config.Service{Name: "q", Dir: "q"},
			want:  nil,
		},
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
				t.Fatalf("SniffPorts = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("SniffPorts = %v, want %v", got, tt.want)
				}
			}
		})
	}
}
