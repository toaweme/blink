package format

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"

	"github.com/toaweme/blink/core/config"
)

type Format string

const (
	FormatJSON Format = "json"
	FormatYAML Format = "yaml"
	FormatTOML Format = "toml"
)

type Writer interface {
	Write(config config.Config, format Format) error
}

type writer struct {
	file string
}

func NewWriter(file string) Writer {
	return &writer{file: file}
}

func (w *writer) Write(config config.Config, format Format) error {
	err := write(w.file, config, format)
	if err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	return nil
}

func write(file string, cfg config.Config, format Format) error {
	var data []byte
	var err error

	// create dirs if they don't exist
	if err = os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return fmt.Errorf("failed to create directories in: %s: %w", file, err)
	}

	switch format {
	case FormatJSON:
		data, err = json.MarshalIndent(cfg, "", "  ")
	case FormatYAML:
		// Step 1: Marshal the struct into YAML bytes.
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

	if err = os.WriteFile(file, data, 0o644); err != nil {
		return fmt.Errorf("writing to file: %w", err)
	}

	return nil
}

// ReformatKeyValuesAsInlineArray accepts a YAML byte slice and a key name (e.g., "extensions").
// It searches the entire YAML structure for the key and formats its values as inline arrays.
func reformatKeyValuesAsInlineArray(yamlBytes []byte, key string) ([]byte, error) {
	var root yaml.Node
	err := yaml.Unmarshal(yamlBytes, &root)
	if err != nil {
		return nil, err
	}

	// Ensure there's content to work with
	if len(root.Content) == 0 {
		return nil, fmt.Errorf("empty YAML content")
	}
	node := root.Content[0] // The root node

	// Modify all sequence nodes with the specified key to have FlowStyle
	setSequenceToFlowStyle(node, key)

	// Marshal the modified YAML back to bytes
	out, err := yaml.Marshal(&root)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// setSequenceToFlowStyle recursively traverses the YAML node tree to find all occurrences of the target key.
// Once found, it sets the sequence node's style to FlowStyle to make it an inline array.
func setSequenceToFlowStyle(node *yaml.Node, key string) {
	switch node.Kind {
	case yaml.MappingNode:
		// Iterate over key-value pairs in the mapping node
		for i := 0; i < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valueNode := node.Content[i+1]

			// Check if the current key matches the target key
			if keyNode.Value == key {
				// If the value is a sequence node, set its style to FlowStyle
				if valueNode.Kind == yaml.SequenceNode {
					valueNode.Style = yaml.FlowStyle
				}
			}

			// Recurse into the value node
			setSequenceToFlowStyle(valueNode, key)
		}
	case yaml.SequenceNode:
		// Recurse into each element of the sequence
		for _, n := range node.Content {
			setSequenceToFlowStyle(n, key)
		}
	default:
		// DocumentNode, AliasNode, ScalarNode - recurse into their
		// content if any.
		for _, n := range node.Content {
			setSequenceToFlowStyle(n, key)
		}
	}
}
