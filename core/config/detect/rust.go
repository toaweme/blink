package detect

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"

	"github.com/toaweme/blink/core/config"
)

// rustDetector recognises a Cargo project and emits a shell-runtime service
// that builds and runs the crate via cargo.
type rustDetector struct{}

var _ Detector = rustDetector{}

func (rustDetector) Name() string { return "rust" }

func (rustDetector) Detect(dir string) ([]Detected, error) {
	path := filepath.Join(dir, "Cargo.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read Cargo.toml: %w", err)
	}

	var manifest struct {
		Package struct {
			Name string `toml:"name"`
		} `toml:"package"`
	}
	if err := toml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse Cargo.toml: %w", err)
	}

	name := manifest.Package.Name
	if name == "" {
		name = filepath.Base(dir)
	}

	svc := config.Service{
		Name:    name,
		Runtime: "shell",
		Commands: config.Commands{
			Build: &config.Command{Command: "cargo build"},
			Run:   &config.Command{Command: "cargo run", Service: true},
		},
		Fs:     config.Fs{Extensions: []string{"rs", "toml"}},
		Reload: config.Reload{Reload: true},
	}
	return []Detected{{
		Service: svc,
		Source:  "rust",
		File:    "Cargo.toml",
		Label:   name + " (rust)",
	}}, nil
}
