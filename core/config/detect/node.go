package detect

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/toaweme/blink/core/config"
)

// nodeDetector recognizes a Node project (package.json) and emits a single
// node-runtime service. It picks the dev script (falling back to start) and
// labels the service with the detected framework when it recognizes one.
type nodeDetector struct{}

var _ Detector = nodeDetector{}

func (nodeDetector) Name() string { return "node" }

type packageJSON struct {
	Name            string            `json:"name"`
	Scripts         map[string]string `json:"scripts"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}

func (nodeDetector) Detect(dir string) ([]Detected, error) {
	path := filepath.Join(dir, "package.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// absence of package.json is a no-match, not a failure.
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read package.json: %w", err)
	}
	var pkg packageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, fmt.Errorf("failed to parse package.json: %w", err)
	}

	name := pkg.Name
	if name == "" {
		name = filepath.Base(dir)
	}

	label := name + " (node)"
	if fw := detectFramework(pkg); fw != "" {
		label = name + " (" + fw + ")"
	}

	svc := config.Service{
		Name:    name,
		Runtime: "node",
		Node: &config.NodeConfig{
			Script:         pickScript(pkg.Scripts),
			PackageManager: config.NodePackageManager(dir),
		},
		Reload: config.Reload{Reload: true},
	}
	return []Detected{{
		Service: svc,
		Source:  "node",
		File:    "package.json",
		Label:   label,
	}}, nil
}

// pickScript prefers "dev", then "start"; empty when neither exists (the node
// runtime defaults to "dev" itself, so an empty value is harmless).
func pickScript(scripts map[string]string) string {
	for _, name := range []string{"dev", "start"} {
		if _, ok := scripts[name]; ok {
			return name
		}
	}
	return ""
}

// detectFramework returns a recognized dev-server framework from the merged
// dependency sets, for labeling only. Empty when none match.
func detectFramework(pkg packageJSON) string {
	for _, fw := range []string{"next", "astro", "nuxt", "remix", "@sveltejs/kit", "vite"} {
		if _, ok := pkg.Dependencies[fw]; ok {
			return frameworkLabel(fw)
		}
		if _, ok := pkg.DevDependencies[fw]; ok {
			return frameworkLabel(fw)
		}
	}
	return ""
}

func frameworkLabel(dep string) string {
	switch dep {
	case "@sveltejs/kit":
		return "sveltekit"
	case "next":
		return "next.js"
	default:
		return dep
	}
}
