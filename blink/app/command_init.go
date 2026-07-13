package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/toaweme/cli"

	"github.com/toaweme/blink/blink/internal/configform"
	"github.com/toaweme/blink/core/addon"
	"github.com/toaweme/blink/core/config"
	"github.com/toaweme/blink/core/config/detect"
	"github.com/toaweme/blink/core/config/format"
)

// InitConfig holds the flags the init command accepts.
type InitConfig struct {
	Config string `arg:"config" short:"c" env:"BLINK_CONFIG" default:"blink.yml" help:"Config file to create; extension picks the format (.yml/.yaml, .toml, .json)."`
	Force  bool   `arg:"force"  short:"f" help:"Overwrite an existing file."`
}

// InitCommand scans the project and interactively creates a new blink.yml.
type InitCommand struct {
	cli.BaseCommand[InitConfig]
	reg *addon.Registry
}

var _ cli.Command[InitConfig] = (*InitCommand)(nil)

// NewInitCommand builds the init command using reg for the picker's port-probe
// key, which spins a service up to observe its real ports.
func NewInitCommand(reg *addon.Registry) *InitCommand {
	return &InitCommand{BaseCommand: cli.NewBaseCommand[InitConfig](), reg: reg}
}

// Run scans the project, runs the interactive picker, and writes the selected
// services to a new blink.yml.
func (c *InitCommand) Run(options cli.GlobalFlags, _ cli.Unknowns) error {
	target := c.Inputs.Config
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

	// cancel any background probe still running when init returns.
	probeCtx, cancelProbes := context.WithCancel(context.Background())
	defer cancelProbes()
	probeFn := func(svc config.Service) ([]config.Port, error) {
		return runtimeProbe(probeCtx, c.reg, options.Cwd, svc)
	}

	// add-from-path (`f`) scans a sibling directory and rebases its services'
	// Dir against the project root.
	scanPathFn := func(path string) ([]config.Service, error) {
		return scanServicesAt(options.Cwd, path)
	}

	kept, err := configform.PickServices("blink init", services, configform.PickOptions{
		ScanPathFn: scanPathFn,
		ProbeFn:    probeFn,
	})
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
	trimWriteDefaults(options.Cwd, &cfg)
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

	// only sniff ports from a .env that belongs to a single service's own
	// directory. When several services share a dir (e.g. all running from the
	// monorepo root), that dir's .env can't be attributed to one of them, so its
	// ports would tag them all identically.
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

// scanServicesAt scans target (an absolute path, or one relative to
// projectRoot) for services and rebases each service's Dir to be relative to
// projectRoot, so services discovered outside the project root run from the
// right place. It backs the init/edit picker's add-from-path action, letting a
// single blink.yaml supervise repos that live in sibling directories.
func scanServicesAt(projectRoot, target string) ([]config.Service, error) {
	if strings.TrimSpace(target) == "" {
		return nil, errors.New("no directory given")
	}
	abs := target
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(projectRoot, target)
	}
	abs = filepath.Clean(abs)

	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %q: %w", target, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%q is not a directory", target)
	}

	// scanServices sniffs ports against abs, so it must run before the rebase
	// while each service's Dir is still relative to the scanned root.
	services, err := scanServices(abs)
	if err != nil {
		return nil, err
	}

	rel, err := filepath.Rel(projectRoot, abs)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve %q relative to the project root: %w", abs, err)
	}
	for i := range services {
		dir := filepath.ToSlash(filepath.Join(rel, services[i].Dir))
		if dir == "." {
			dir = ""
		}
		services[i].Dir = dir
	}
	return services, nil
}

// trimWriteDefaults drops config that merely echoes what blink would detect
// anyway, so a default selection produces a minimal blink.yaml. Currently it
// clears a docker service whose recorded Services cover the whole compose file:
// an empty list already means "run the entire stack", so naming every container
// adds nothing. A genuine hand-authored subset is shorter than the full set and
// is left untouched.
func trimWriteDefaults(cwd string, cfg *config.Config) {
	for i := range cfg.Services {
		svc := &cfg.Services[i]
		if svc.Runtime != "docker" || svc.Docker == nil {
			continue
		}
		if len(svc.Docker.Services) > 0 {
			if full, err := detect.ComposeServices(cwd, *svc); err == nil && coversAll(svc.Docker.Services, full) {
				svc.Docker.Services = nil
			}
		}
		// an all-default docker block adds nothing over `runtime: docker`; drop the
		// pointer so it isn't written as an empty `docker: {}`.
		if svc.Docker.IsZero() {
			svc.Docker = nil
		}
	}
}

// coversAll reports whether subset names exactly the same set as full,
// order-independent (the recorded list is the whole stack).
func coversAll(subset, full []string) bool {
	if len(full) == 0 || len(subset) != len(full) {
		return false
	}
	have := make(map[string]bool, len(subset))
	for _, s := range subset {
		have[s] = true
	}
	for _, f := range full {
		if !have[f] {
			return false
		}
	}
	return true
}

// writeConfig serializes cfg via the shared format writer so init and edit
// produce identical output. The format is chosen from the path's extension
// (.yml/.yaml, .toml, .json), so callers pick a format by naming the file;
// init defaults to blink.yml and edit reuses the loaded file's extension. An
// unrecognized or missing extension falls back to YAML.
func writeConfig(path string, cfg config.Config) error {
	cfg.Runtime = config.RuntimeOptions{}
	f, err := format.ForPath(path)
	if err != nil {
		f = format.FormatYAML
	}
	if err := format.NewWriter(path).Write(cfg, f); err != nil {
		return fmt.Errorf("failed to write %q: %w", path, err)
	}
	return nil
}

// Validate reports whether the parsed flags are valid.
func (c *InitCommand) Validate(_ map[string]any) error { return nil }

// Help returns the one-line description shown in the command list.
func (c *InitCommand) Help() string {
	return "Scan the project and interactively create a blink.yml in the current directory."
}
