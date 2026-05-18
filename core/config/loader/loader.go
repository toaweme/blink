package loader

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	clicfg "github.com/toaweme/cli/config"

	"github.com/toaweme/blink/core/config"
)

// configNames are searched in order when discovering a config from cwd upwards.
var configNames = []string{".blink.yml", "blink.yml", ".blink.yaml", "blink.yaml"}

// Load returns the parsed config and the absolute path it was read from.
//
// If explicit is non-empty, the path is used directly. Otherwise the function
// walks up from start looking for one of configNames. If DirRoot is unset in
// the loaded file, it defaults to the directory containing the config.
func Load(start, explicit string) (config.Config, string, error) {
	path := explicit
	if path == "" {
		found, err := Discover(start)
		if err != nil {
			return config.Config{}, "", err
		}
		path = found
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return config.Config{}, "", fmt.Errorf("failed to resolve config path: %w", err)
	}

	data, err := os.ReadFile(abs)
	if err != nil {
		return config.Config{}, "", fmt.Errorf("failed to read %s: %w", abs, err)
	}

	var cfg config.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return config.Config{}, "", fmt.Errorf("failed to parse %s: %w", abs, err)
	}

	if cfg.DirRoot == "" {
		cfg.DirRoot = filepath.Dir(abs)
	} else if !filepath.IsAbs(cfg.DirRoot) {
		cfg.DirRoot = filepath.Join(filepath.Dir(abs), cfg.DirRoot)
	}

	cfg.ConfigPath = abs
	cfg.Paths.Resolve(cfg.DirRoot)

	if err := Validate(cfg); err != nil {
		return config.Config{}, abs, fmt.Errorf("invalid config %s: %w", abs, err)
	}

	return cfg, abs, nil
}

// Discover walks up from start looking for a blink config file. Returns the
// absolute path. Returns os.ErrNotExist (wrapped) if nothing is found.
func Discover(start string) (string, error) {
	path := clicfg.Discover(start, configNames)
	if path == "" {
		return "", fmt.Errorf("no blink.yaml found from %s: %w", start, os.ErrNotExist)
	}
	return path, nil
}

// Validate checks structural invariants the supervisor relies on.
func Validate(cfg config.Config) error {
	if len(cfg.Services) == 0 {
		return fmt.Errorf("no services defined")
	}

	seen := make(map[string]struct{}, len(cfg.Services))
	for _, s := range cfg.Services {
		if s.Name == "" {
			return fmt.Errorf("service with empty name")
		}
		if _, dup := seen[s.Name]; dup {
			return fmt.Errorf("duplicate service name: %s", s.Name)
		}
		seen[s.Name] = struct{}{}
	}

	for _, s := range cfg.Services {
		for _, dep := range s.Reload.ReloadOnService {
			if _, ok := seen[dep]; !ok {
				return fmt.Errorf("service %q depends on unknown service %q", s.Name, dep)
			}
			if dep == s.Name {
				return fmt.Errorf("service %q depends on itself", s.Name)
			}
		}
	}

	return nil
}
