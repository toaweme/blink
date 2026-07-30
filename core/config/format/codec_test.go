package format

import (
	"reflect"
	"strings"
	"testing"

	"github.com/toaweme/blink/core/config"
)

func Test_YAMLCodec_Marshal_Compact(t *testing.T) {
	cfg := config.Config{
		Services: []config.Service{
			{
				Name:    "api",
				Runtime: "go",
				Go:      &config.GoConfig{Package: "./cmd/api", Args: []string{"serve", "--dev"}},
				Fs:      config.Fs{Include: []string{"package.json"}},
				Reload: config.Reload{
					Reload:         true,
					ReloadOnDelete: []string{"node_modules"},
				},
				Ports: []config.Port{config.LiteralPort(4000), config.LiteralPort(4001)},
			},
			{Name: "worker", Runtime: "go"},
		},
	}

	data, err := (yamlCodec{}).Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal YAML config: %v", err)
	}

	want := `services:
  - name: api
    runtime: go
    go:
      package: ./cmd/api
      args: [serve, --dev]
    fs:
      include: [package.json]
    reload:
      reload: true
      reload_on_delete: [node_modules]
    ports: [4000, 4001]

  - name: worker
    runtime: go
`
	if string(data) != want {
		t.Fatalf("unexpected YAML config\ngot:\n%s\nwant:\n%s", data, want)
	}

	var decoded config.Config
	if err := (yamlCodec{}).Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal compact YAML config: %v", err)
	}
	if !reflect.DeepEqual(decoded, cfg) {
		t.Fatalf("round trip changed config\ngot: %#v\nwant: %#v", decoded, cfg)
	}
}

func Test_YAMLCodec_Marshal_KeepsMappingSequencesExpanded(t *testing.T) {
	cfg := config.Config{
		Services: []config.Service{
			{
				Name: "api",
				Commands: config.Commands{
					Setup: []config.Command{{Command: "go generate ./..."}},
				},
			},
		},
	}

	data, err := (yamlCodec{}).Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal YAML config: %v", err)
	}

	if strings.Contains(string(data), "setup: [{") {
		t.Fatalf("mapping sequence was formatted inline:\n%s", data)
	}
}
