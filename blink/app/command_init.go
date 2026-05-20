package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/toaweme/cli"

	"github.com/toaweme/blink/blink/internal/configform"
	"github.com/toaweme/blink/core/config"
	"github.com/toaweme/blink/core/config/detect"
	"github.com/toaweme/blink/core/config/format"
)

type InitConfig struct {
	Path  string `arg:"path"  short:"p" env:"BLINK_INIT_PATH"  default:"blink.yaml" help:"Output path."`
	Force bool   `arg:"force" short:"f" env:"BLINK_INIT_FORCE"                     help:"Overwrite an existing file."`
}

type InitCommand struct {
	cli.BaseCommand[InitConfig]
}

var _ cli.Command[InitConfig] = (*InitCommand)(nil)

func NewInitCommand() *InitCommand {
	return &InitCommand{BaseCommand: cli.NewBaseCommand[InitConfig]()}
}

func (c *InitCommand) Run(options cli.GlobalFlags, _ cli.Unknowns) error {
	target := c.Inputs.Path
	if !filepath.IsAbs(target) {
		target = filepath.Join(options.Cwd, target)
	}

	if _, err := os.Stat(target); err == nil && !c.Inputs.Force {
		return fmt.Errorf("file %q already exists, use --force or run `blink edit` instead", target)
	}

	services, err := scanServices(options.Cwd)
	if err != nil {
		return err
	}

	kept, err := configform.PickServices("blink init", services, nil)
	if err != nil {
		if errors.Is(err, configform.ErrCanceled) {
			fmt.Println("aborted, nothing written")
			return nil
		}
		return err
	}
	if len(kept) == 0 {
		fmt.Println("nothing to write (no services kept)")
		return nil
	}

	cfg := config.Config{Services: kept}
	if err := writeConfig(target, cfg); err != nil {
		return err
	}
	fmt.Println("wrote", target)
	return nil
}

// scanServices runs detection and fills in a best-effort port guess for any
// service that didn't already declare one, so the picker shows ports up front.
func scanServices(cwd string) ([]config.Service, error) {
	_, detected, err := detect.Scan(cwd)
	if err != nil {
		return nil, fmt.Errorf("failed to scan project: %w", err)
	}
	services := make([]config.Service, 0, len(detected))
	for _, d := range detected {
		services = append(services, d.Service)
	}

	// Only sniff ports from a .env that belongs to a single service's own
	// directory. When several services share a dir (a monorepo where every
	// service runs from the root), that dir's .env can't be attributed to one
	// of them, so attaching its ports would tag them all identically.
	dirCount := make(map[string]int, len(services))
	for _, s := range services {
		dirCount[s.Dir]++
	}
	for i := range services {
		if len(services[i].Ports) == 0 && dirCount[services[i].Dir] == 1 {
			services[i].Ports = detect.SniffPorts(cwd, services[i])
		}
	}
	return services, nil
}

// writeConfig serializes cfg to path as YAML via the shared format writer so
// init and edit produce identical output (inline extension arrays, omitted
// transient fields).
func writeConfig(path string, cfg config.Config) error {
	cfg.Runtime = config.RuntimeOptions{}
	if err := format.NewWriter(path).Write(cfg, format.FormatYAML); err != nil {
		return fmt.Errorf("failed to write %q: %w", path, err)
	}
	return nil
}

func (c *InitCommand) Validate(_ map[string]any) error { return nil }

func (c *InitCommand) Help() string {
	return "Scan the project and interactively create a blink.yaml in the current directory."
}
