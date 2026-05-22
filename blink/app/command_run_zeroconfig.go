package app

import (
	"errors"
	"fmt"
	"os"

	"github.com/mattn/go-isatty"

	"github.com/toaweme/blink/blink/internal/configform"
	"github.com/toaweme/blink/core/config"
	"github.com/toaweme/blink/core/config/detect"
)

// zeroConfig builds an ephemeral Config by scanning cwd and letting the user
// pick which detected services to run. Nothing is written to disk; `blink init`
// persists the detected setup. Paths are resolved here because the loader that
// normally does it was skipped.
func zeroConfig(cwd string) (config.Config, error) {
	cfg, detected, err := detect.Scan(cwd)
	if err != nil {
		return config.Config{}, fmt.Errorf("failed to scan project: %w", err)
	}
	if len(detected) == 0 {
		return config.Config{}, fmt.Errorf("no blink.yaml found and nothing detected in %s; run `blink init`", cwd)
	}

	// the picker is interactive; without a TTY (CI, piped) error out instead of hanging on a form.
	if !isTTY() {
		return config.Config{}, fmt.Errorf("no blink.yaml found in %s; run `blink init` to create one", cwd)
	}

	fmt.Fprintln(os.Stderr, "no blink.yaml found - running detected services. run `blink init` to save a config.")

	chosen, err := configform.PickDetected(detected)
	if err != nil {
		return config.Config{}, err
	}
	if len(chosen) == 0 {
		return config.Config{}, errors.New("no services selected")
	}
	cfg.Services = chosen
	cfg.Paths.Resolve(cfg.DirRoot)
	return cfg, nil
}

func isTTY() bool {
	fd := os.Stdout.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}
