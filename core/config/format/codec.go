package format

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"

	clicfg "github.com/toaweme/cli/config"
)

// ErrEmptyConfig reports a config that marshaled to no YAML document at all,
// which leaves the formatter with no root node to work on.
var ErrEmptyConfig = errors.New("empty config")

// Codec marshals and unmarshals a Config in one on-disk format. It is the
// cli/config codec contract, so the same value satisfies that package's Store
// codec slot. blink implements it with the libraries already in go.mod
// (pelletier/go-toml/v2, yaml.v3, encoding/json) rather than the cli addon
// codecs, which would pull in a second TOML library.
type Codec = clicfg.Codec

type jsonCodec struct{}

type yamlCodec struct{}

type tomlCodec struct{}

var (
	_ Codec = jsonCodec{}
	_ Codec = yamlCodec{}
	_ Codec = tomlCodec{}
)

func (jsonCodec) Marshal(v any) ([]byte, error)      { return json.MarshalIndent(v, "", "  ") }
func (jsonCodec) Unmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }
func (jsonCodec) Extension() string                  { return ".json" }

func (yamlCodec) Marshal(v any) ([]byte, error) {
	data, err := yaml.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal YAML config: %w", err)
	}
	return compactYAML(data)
}
func (yamlCodec) Unmarshal(data []byte, v any) error { return yaml.Unmarshal(data, v) }
func (yamlCodec) Extension() string                  { return ".yml" }

func (tomlCodec) Marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
func (tomlCodec) Unmarshal(data []byte, v any) error { return toml.Unmarshal(data, v) }
func (tomlCodec) Extension() string                  { return ".toml" }

// codecFor returns the codec for a Format.
func codecFor(f Format) (Codec, error) {
	switch f {
	case FormatJSON:
		return jsonCodec{}, nil
	case FormatYAML:
		return yamlCodec{}, nil
	case FormatTOML:
		return tomlCodec{}, nil
	default:
		return nil, fmt.Errorf("unsupported format: %s", f)
	}
}

// ForPath infers the on-disk Format from a file's extension.
func ForPath(path string) (Format, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return FormatJSON, nil
	case ".yml", ".yaml":
		return FormatYAML, nil
	case ".toml":
		return FormatTOML, nil
	default:
		return "", fmt.Errorf("unsupported config extension %q", filepath.Ext(path))
	}
}

// CodecForPath returns the codec to decode or encode the file at path, chosen
// by its extension.
func CodecForPath(path string) (Codec, error) {
	f, err := ForPath(path)
	if err != nil {
		return nil, err
	}
	return codecFor(f)
}

// compactYAML keeps scalar lists on one line and separates service entries.
func compactYAML(yamlBytes []byte) ([]byte, error) {
	var root yaml.Node
	err := yaml.Unmarshal(yamlBytes, &root)
	if err != nil {
		return nil, fmt.Errorf("failed to decode YAML syntax tree: %w", err)
	}

	if len(root.Content) == 0 {
		return nil, fmt.Errorf("failed to format YAML config: %w", ErrEmptyConfig)
	}

	setScalarSequencesToFlowStyle(root.Content[0])

	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(&root); err != nil {
		return nil, fmt.Errorf("failed to encode compact YAML config: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("failed to finish compact YAML config: %w", err)
	}

	return separateTopLevelSequenceEntries(out.Bytes()), nil
}

// setScalarSequencesToFlowStyle recursively inlines lists whose values are all scalars.
func setScalarSequencesToFlowStyle(node *yaml.Node) {
	if node.Kind == yaml.SequenceNode && len(node.Content) > 0 {
		scalarOnly := true
		for _, child := range node.Content {
			if child.Kind != yaml.ScalarNode && child.Kind != yaml.AliasNode {
				scalarOnly = false
				break
			}
		}
		if scalarOnly {
			node.Style = yaml.FlowStyle
		}
	}

	for _, child := range node.Content {
		setScalarSequencesToFlowStyle(child)
	}
}

// separateTopLevelSequenceEntries inserts a blank line between block entries nested directly under a root key.
func separateTopLevelSequenceEntries(data []byte) []byte {
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	var out strings.Builder
	seenEntry := false
	for _, line := range lines {
		if strings.HasPrefix(line, "  - ") {
			if seenEntry {
				out.WriteByte('\n')
			}
			seenEntry = true
		}
		out.WriteString(line)
		out.WriteByte('\n')
	}
	return []byte(out.String())
}
