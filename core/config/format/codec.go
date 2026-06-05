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
		return nil, err
	}
	return reformatKeyValuesAsInlineArray(data, "extensions")
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
