package format

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/toaweme/blink/core/config"
)

func TestWriterFormats(t *testing.T) {
	cfg := config.Config{
		Services: []config.Service{
			{Name: "api", Runtime: "go", Ports: []int{8080}},
			{Name: "db", Runtime: "docker"},
		},
	}

	tests := []struct {
		format  Format
		ext     string
		contain string
	}{
		{FormatYAML, "blink.yaml", "name: api"},
		{FormatJSON, "blink.json", `"name": "api"`},
		{FormatTOML, "blink.toml", `name = 'api'`},
	}

	dir := t.TempDir()
	for _, tc := range tests {
		t.Run(string(tc.format), func(t *testing.T) {
			path := filepath.Join(dir, tc.ext)
			w := NewWriter(path)
			require.NoError(t, w.Write(cfg, tc.format))
			b, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Contains(t, string(b), tc.contain)
		})
	}
}
