// Package loader reads and resolves a blink config file into a Config.
package loader

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	clicfg "github.com/toaweme/cli/config"

	"github.com/toaweme/blink/core/config"
	"github.com/toaweme/blink/core/config/format"
)

// configNames are searched in order when discovering a config from cwd upwards.
// blink.yml is canonical (what `blink init` writes); the rest are accepted
// fallbacks ordered yml/yaml before toml before json. Each name is decoded by
// the codec its extension implies (see format.CodecForPath).
var configNames = []string{"blink.yml", "blink.yaml", "blink.toml", "blink.json"}

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

	codec, err := format.CodecForPath(abs)
	if err != nil {
		return config.Config{}, "", fmt.Errorf("failed to select codec for %s: %w", abs, err)
	}

	var cfg config.Config
	if err := codec.Unmarshal(data, &cfg); err != nil {
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
		return "", fmt.Errorf("no blink config (yml/yaml/toml/json) found from %s: %w", start, os.ErrNotExist)
	}
	return path, nil
}

// Validate checks structural invariants the supervisor relies on.
func Validate(cfg config.Config) error {
	if len(cfg.Services) == 0 {
		return errors.New("no services defined")
	}

	seen := make(map[string]struct{}, len(cfg.Services))
	for _, s := range cfg.Services {
		if s.Name == "" {
			return errors.New("service with empty name")
		}
		if _, dup := seen[s.Name]; dup {
			return fmt.Errorf("duplicate service name: %s", s.Name)
		}
		seen[s.Name] = struct{}{}
	}

	for _, s := range cfg.Services {
		if s.Node != nil && !config.ValidPackageManager(s.Node.PackageManager) {
			return fmt.Errorf("service %q has unknown package_manager %q (want npm/pnpm/yarn/bun): %w", s.Name, s.Node.PackageManager, config.ErrInvalidConfig)
		}
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
