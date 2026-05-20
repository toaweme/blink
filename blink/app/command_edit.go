package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/toaweme/cli"

	"github.com/toaweme/blink/blink/internal/configform"
	"github.com/toaweme/blink/core/addon"
	"github.com/toaweme/blink/core/config"
	"github.com/toaweme/blink/core/config/loader"
)

type EditConfig struct {
	Config string `arg:"config" short:"c" env:"BLINK_CONFIG" help:"Path to blink.yaml. Walks up from cwd when empty."`
}

type EditCommand struct {
	cli.BaseCommand[EditConfig]
	reg *addon.Registry
}

var _ cli.Command[EditConfig] = (*EditCommand)(nil)

// NewEditCommand builds the edit command. reg supplies the runtimes used by the
// picker's port-probe key.
func NewEditCommand(reg *addon.Registry) *EditCommand {
	return &EditCommand{BaseCommand: cli.NewBaseCommand[EditConfig](), reg: reg}
}

func (c *EditCommand) Run(options cli.GlobalFlags, _ cli.Unknowns) error {
	cfg, path, err := loader.Load(options.Cwd, c.Inputs.Config)
	if err != nil {
		return fmt.Errorf("failed to load config (run `blink init` first?): %w", err)
	}

	// re-detect merges any newly added services into the picker on `d`.
	detectFn := func() ([]config.Service, error) { return scanServices(options.Cwd) }

	// cancel any probe still running in the background when edit returns.
	probeCtx, cancelProbes := context.WithCancel(context.Background())
	defer cancelProbes()
	probeFn := func(svc config.Service) ([]config.Port, error) {
		return runtimeProbe(probeCtx, c.reg, options.Cwd, svc)
	}

	kept, err := configform.PickServices("blink edit · "+filepath.Base(path), cfg.Services, detectFn, probeFn)
	if err != nil {
		if errors.Is(err, configform.ErrCanceled) {
			fmt.Println("aborted, no changes written")
			return nil
		}
		return err
	}

	cfg.Services = kept
	trimWriteDefaults(options.Cwd, &cfg)
	if err := writeConfig(path, cfg); err != nil {
		return err
	}
	fmt.Println("saved", path)
	return nil
}

func (c *EditCommand) Validate(_ map[string]any) error { return nil }

func (c *EditCommand) Help() string {
	return "Interactively add, remove, or modify entries in an existing blink.yaml."
}
