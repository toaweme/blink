package app

import (
	"errors"
	"fmt"
	"os"

	"github.com/toaweme/cli"

	"github.com/toaweme/blink/blink/internal/ui"
	"github.com/toaweme/blink/core/addon"
	"github.com/toaweme/blink/core/config"
	"github.com/toaweme/blink/core/config/loader"
	"github.com/toaweme/blink/core/control"
)

// RunConfig holds the flags the supervise-services command accepts.
type RunConfig struct {
	Config        string `arg:"config"         short:"c" env:"BLINK_CONFIG"         help:"Path to the blink config (yml/yaml/toml/json). Walks up from cwd when empty."`
	UI            string `arg:"ui"             short:"u" env:"BLINK_UI"             help:"Override UI: blink, plain, headless (alias: none)."`
	Zen           bool   `arg:"zen"            short:"z" env:"BLINK_ZEN"            help:"Start the TUI in zen mode (native scrollback, no chrome)."`
	ForceShutdown string `arg:"force-shutdown" short:"k" env:"BLINK_FORCE_SHUTDOWN" help:"Override force-shutdown: on (kill anything bound to declared ports before start), off (never). Default on."`
	Logs          string `arg:"logs"           short:"l" env:"BLINK_LOGS"           help:"Override log writing: on (write <LogDir>/<svc>.log), off (write nothing). Overrides the config's logs.write. Default on."`

	Services []string `arg:"services" short:"s" sep:"," env:"BLINK_SERVICES" help:"Comma-separated list of services to start (subset of the config). Empty starts all."`
}

// RunCommand supervises the services defined in blink.yaml with live reload.
type RunCommand struct {
	cli.BaseCommand[RunConfig]
	reg *addon.Registry
}

var _ cli.Command[RunConfig] = (*RunCommand)(nil)

// NewRunCommand builds the run command using reg to supervise services.
func NewRunCommand(reg *addon.Registry) *RunCommand {
	return &RunCommand{BaseCommand: cli.NewBaseCommand[RunConfig](), reg: reg}
}

// Run loads the config, applies the run flags, and starts the supervised UI.
func (c *RunCommand) Run(options cli.GlobalFlags, _ cli.Unknowns) error {
	cfg, err := loadRunConfig(options.Cwd, *c.Inputs)
	if err != nil {
		return err
	}
	return runUI(cfg, c.reg)
}

// Validate reports whether the parsed flags are valid.
func (c *RunCommand) Validate(_ map[string]any) error { return nil }

// Help returns the one-line description shown in the command list.
func (c *RunCommand) Help() string {
	return "Run the services defined in blink.yaml with live reload."
}

// loadRunConfig loads blink.yaml and applies the run flags. When no config is
// found (and none was named with -c), it falls back to zero-config: scan, let
// the user pick detected services, and run them ephemerally without writing a
// file. An explicitly named but missing config, or a parse error, fails hard.
func loadRunConfig(cwd string, in RunConfig) (config.Config, error) {
	cfg, _, err := loader.Load(cwd, in.Config)
	if err != nil {
		if in.Config != "" || !errors.Is(err, os.ErrNotExist) {
			return config.Config{}, err
		}
		zc, zerr := zeroConfig(cwd)
		if zerr != nil {
			return config.Config{}, zerr
		}
		cfg = zc
	}

	if in.UI != "" {
		cfg.UI = in.UI
	}
	switch in.ForceShutdown {
	case "on", "true", "yes":
		t := true
		cfg.ForceShutdown = &t
	case "off", "false", "no":
		f := false
		cfg.ForceShutdown = &f
	}
	switch in.Logs {
	case "on", "true", "yes":
		t := true
		cfg.Logs.Write = &t
	case "off", "false", "no":
		f := false
		cfg.Logs.Write = &f
	}
	cfg.Zen = in.Zen

	// disabled services stay in the config but never run: drop them before
	// scoping so they don't appear in the supervised set or the TUI, and so a
	// --services reference to a disabled service fails fast like an unknown one.
	cfg.Services = enabledServices(cfg.Services)

	// the cli lib splits the comma-separated flag/env via the field's sep tag.
	cfg.Runtime.Services = in.Services

	// --services scopes the supervised set. Validate names against the loaded
	// yaml so a typo fails fast instead of silently starting nothing.
	if len(cfg.Runtime.Services) > 0 {
		scoped, err := scopeServices(cfg.Services, cfg.Runtime.Services)
		if err != nil {
			return cfg, err
		}
		cfg.Services = scoped
	}

	// validate control.keys here so a typo fails fast for every UI mode
	// (headless / plain never build a keymap, so they'd otherwise skip it).
	if _, err := control.DefaultKeymap().Merge(cfg.Control.Keys); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// runUI builds the UI registry and runs the configured backend.
func runUI(cfg config.Config, reg *addon.Registry) error {
	app := ui.NewApp(ui.DefaultRegistry(reg))
	return app.Run(cfg)
}

// enabledServices returns only the services that are not marked Disabled, so a
// deselected-but-kept service (see config.Service.Disabled) is excluded from the
// run without being removed from the config file.
func enabledServices(all []config.Service) []config.Service {
	out := make([]config.Service, 0, len(all))
	for _, s := range all {
		if !s.Disabled {
			out = append(out, s)
		}
	}
	return out
}

// scopeServices filters the configured service list to the names passed via
// --services. Unknown names are an error so a typo can't silently launch nothing.
func scopeServices(all []config.Service, want []string) ([]config.Service, error) {
	idx := make(map[string]config.Service, len(all))
	for _, s := range all {
		idx[s.Name] = s
	}
	out := make([]config.Service, 0, len(want))
	for _, name := range want {
		if name == "" {
			// tolerate a trailing or doubled comma (web, or web,,api).
			continue
		}
		s, ok := idx[name]
		if !ok {
			return nil, fmt.Errorf("--services references unknown service %q", name)
		}
		out = append(out, s)
	}
	return out, nil
}
