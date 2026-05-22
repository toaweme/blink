// Package format serializes a blink Config to disk in JSON, YAML, or TOML.
package format

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"

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
	var data []byte
	var err error

	if err = os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return fmt.Errorf("failed to create directories in: %s: %w", file, err)
	}

	switch format {
	case FormatJSON:
		data, err = json.MarshalIndent(cfg, "", "  ")
	case FormatYAML:
		yamlBytes, err := yaml.Marshal(cfg)
		if err != nil {
			return fmt.Errorf("marshaling config to YAML: %w", err)
		}

		data, err = reformatKeyValuesAsInlineArray(yamlBytes, "extensions")
		if err != nil {
			return fmt.Errorf("reformatting yaml: %w", err)
		}
	case FormatTOML:
		var buf bytes.Buffer
		if err = toml.NewEncoder(&buf).Encode(cfg); err != nil {
			return fmt.Errorf("encoding TOML: %w", err)
		}
		data = buf.Bytes()
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}

	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err = os.WriteFile(file, data, 0o600); err != nil {
		return fmt.Errorf("writing to file: %w", err)
	}

	return nil
}

// reformatKeyValuesAsInlineArray formats every occurrence of key's sequence
// value as an inline (flow-style) array.
func reformatKeyValuesAsInlineArray(yamlBytes []byte, key string) ([]byte, error) {
	var root yaml.Node
	err := yaml.Unmarshal(yamlBytes, &root)
	if err != nil {
		return nil, err
	}

	if len(root.Content) == 0 {
		return nil, errors.New("empty YAML content")
	}
	node := root.Content[0]

	setSequenceToFlowStyle(node, key)

	out, err := yaml.Marshal(&root)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// setSequenceToFlowStyle recursively sets every sequence node reached under key
// to FlowStyle, making it an inline array.
func setSequenceToFlowStyle(node *yaml.Node, key string) {
	switch node.Kind {
	case yaml.MappingNode:
		for i := 0; i < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valueNode := node.Content[i+1]

			if keyNode.Value == key {
				if valueNode.Kind == yaml.SequenceNode {
					valueNode.Style = yaml.FlowStyle
				}
			}

			setSequenceToFlowStyle(valueNode, key)
		}
	case yaml.SequenceNode:
		for _, n := range node.Content {
			setSequenceToFlowStyle(n, key)
		}
	default:
		for _, n := range node.Content {
			setSequenceToFlowStyle(n, key)
		}
	}
}
