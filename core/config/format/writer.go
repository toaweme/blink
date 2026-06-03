// Package format serializes a blink Config to disk in JSON, YAML, or TOML.
package format

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/toaweme/blink/core/config"
)

// Format is the on-disk serialization format for a Config.
type Format string

const (
	// FormatJSON serializes a Config as JSON.
	FormatJSON Format = "json"
	// FormatYAML serializes a Config as YAML.
	FormatYAML Format = "yaml"
	// FormatTOML serializes a Config as TOML.
	FormatTOML Format = "toml"
)

// Writer writes a Config to its target file in a chosen Format.
type Writer interface {
	Write(config config.Config, format Format) error
}

type writer struct {
	file string
}

var _ Writer = (*writer)(nil)

// NewWriter returns a Writer that writes configs to file.
func NewWriter(file string) Writer {
	return &writer{file: file}
}

func (w *writer) Write(cfg config.Config, format Format) error {
	err := write(w.file, cfg, format)
	if err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	return nil
}

func write(file string, cfg config.Config, format Format) error {
	codec, err := codecFor(format)
	if err != nil {
		return err
	}

	if err = os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return fmt.Errorf("failed to create directories in: %s: %w", file, err)
	}

	data, err := codec.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err = os.WriteFile(file, data, 0o600); err != nil {
		return fmt.Errorf("writing to file: %w", err)
	}

	return nil
}
