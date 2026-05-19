package app

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/toaweme/cli"

	"github.com/toaweme/blink/blink/internal/configform"
	"github.com/toaweme/blink/core/config"
	"github.com/toaweme/blink/core/config/loader"
)

type EditConfig struct {
	Config string `arg:"config" short:"c" env:"BLINK_CONFIG" help:"Path to blink.yaml. Walks up from cwd when empty."`
}

type EditCommand struct {
	cli.BaseCommand[EditConfig]
}

var _ cli.Command[EditConfig] = (*EditCommand)(nil)

func NewEditCommand() *EditCommand {
	return &EditCommand{BaseCommand: cli.NewBaseCommand[EditConfig]()}
}

func (c *EditCommand) Run(options cli.GlobalFlags, _ cli.Unknowns) error {
	cfg, path, err := loader.Load(options.Cwd, c.Inputs.Config)
	if err != nil {
		return fmt.Errorf("failed to load config (run `blink init` first?): %w", err)
	}

	// re-detect merges any newly added services into the picker on `d`.
	detectFn := func() ([]config.Service, error) { return scanServices(options.Cwd) }

	kept, err := configform.PickServices("blink edit · "+filepath.Base(path), cfg.Services, detectFn)
	if err != nil {
		if errors.Is(err, configform.ErrCanceled) {
			fmt.Println("aborted, no changes written")
			return nil
		}
		return err
	}

	cfg.Services = kept
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
