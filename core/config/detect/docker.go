package detect

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/toaweme/blink/core/config"
)

// dockerDetector recognises a compose file and emits a single docker-runtime
// service whose DockerConfig.Services lists every compose service, so the
// wizard can checkbox the subset to run. No file-watch is configured: the
// docker runtime manages its containers end to end.
type dockerDetector struct{}

var _ Detector = dockerDetector{}

func (dockerDetector) Name() string { return "docker" }

// composeNames are the conventional compose filenames, in preference order.
var composeNames = []string{"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml"}

func (dockerDetector) Detect(dir string) ([]Detected, error) {
	var file string
	for _, name := range composeNames {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			file = name
			break
		}
	}
	if file == "" {
		return nil, nil
	}

	services, err := composeServices(filepath.Join(dir, file))
	if err != nil {
		return nil, err
	}

	// only record File when it differs from the runtime default, so a stock
	// docker-compose.yml stays implicit rather than echoing the default back.
	dockerCfg := &config.DockerConfig{Services: services}
	if file != config.DefaultComposeFile {
		dockerCfg.File = file
	}
	svc := config.Service{
		Name:    "docker",
		Runtime: "docker",
		Docker:  dockerCfg,
	}
	label := "docker (" + file + ")"
	if n := len(services); n > 0 {
		label = fmt.Sprintf("docker (%d services in %s)", n, file)
	}
	return []Detected{{
		Service: svc,
		Source:  "docker",
		File:    file,
		Label:   label,
	}}, nil
}

// composeServices parses the top-level services map of a compose file and
// returns the service names in declaration order.
func composeServices(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", filepath.Base(path), err)
	}
	var doc struct {
		Services yaml.Node `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("failed to parse compose file %s: %w", filepath.Base(path), err)
	}
	// a mapping node alternates key/value entries in Content; collect the keys.
	var names []string
	for i := 0; i+1 < len(doc.Services.Content); i += 2 {
		names = append(names, doc.Services.Content[i].Value)
	}
	return names, nil
}
