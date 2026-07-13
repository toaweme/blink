package app

import (
	"fmt"
	"io"
	"os"

	"github.com/toaweme/cli"

	"github.com/toaweme/blink/core/config"
)

// NukeConfig holds the flags the nuke command accepts.
type NukeConfig struct {
	Yes    bool `arg:"yes"    short:"y" help:"Skip confirmation prompt."`
	Global bool `arg:"global" short:"g" help:"Also remove user-scoped state (~/.blink), affecting every project."`
}

// NukeCommand removes all resolved blink state and config so the next run
// starts from scratch.
type NukeCommand struct {
	cli.BaseCommand[NukeConfig]
}

var _ cli.Command[NukeConfig] = (*NukeCommand)(nil)

// NewNukeCommand builds the nuke command.
func NewNukeCommand() *NukeCommand {
	return &NukeCommand{BaseCommand: cli.NewBaseCommand[NukeConfig]()}
}

// Run resolves blink's state paths, lists the ones that exist, and removes them
// after confirmation (skipped with -y).
func (c *NukeCommand) Run(_ cli.GlobalFlags, _ cli.Unknowns) error {
	cwd, _ := os.Getwd()
	var p config.Paths
	p.Resolve(cwd)

	confirm := func() bool {
		fmt.Print("continue? [y/N] ")
		var answer string
		// a scan error (e.g. EOF) leaves answer empty, which reads as "no".
		_, _ = fmt.Scanln(&answer)
		return answer == "y" || answer == "Y" || answer == "yes"
	}

	return nuke(p.All(), c.Inputs.Global, c.Inputs.Yes, confirm, os.Stdout)
}

// nuke removes the existing paths among entries, reporting progress to out.
// By default only project-scoped paths (under the current directory) are
// removed; user-scoped paths like ~/.blink are kept unless global is set,
// since wiping them affects every project. Removal is gated on skipConfirm
// (the -y flag) or confirm returning true; otherwise it aborts without
// touching anything. Empty paths are ignored.
func nuke(entries []config.PathEntry, global, skipConfirm bool, confirm func() bool, out io.Writer) error {
	var found, keptUser []config.PathEntry
	for _, e := range entries {
		if e.Path == "" {
			continue
		}
		if _, err := os.Stat(e.Path); err != nil {
			continue
		}
		if e.UserScoped && !global {
			keptUser = append(keptUser, e)
			continue
		}
		found = append(found, e)
	}

	if len(found) == 0 {
		fmt.Fprintln(out, "nothing to remove - blink state is already clean")
		reportKept(out, keptUser)
		return nil
	}

	fmt.Fprintf(out, "the following blink state will be removed:\n\n")
	for _, e := range found {
		fmt.Fprintf(out, "  %s  (%s)\n", e.Path, e.Description)
	}
	fmt.Fprintln(out)
	reportKept(out, keptUser)

	if !skipConfirm && !confirm() {
		fmt.Fprintln(out, "aborted")
		return nil
	}

	var errs []error
	for _, e := range found {
		if err := os.RemoveAll(e.Path); err != nil {
			errs = append(errs, fmt.Errorf("failed to remove %s: %w", e.Path, err))
			fmt.Fprintf(out, "  error: %s\n", err)
		} else {
			fmt.Fprintf(out, "  removed %s\n", e.Path)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to remove %d path(s)", len(errs))
	}

	fmt.Fprintln(out, "\ndone - blink state has been reset")
	return nil
}

// reportKept tells the user which user-scoped paths were left in place and how
// to remove them, so a project-scoped nuke never silently skips global state.
func reportKept(out io.Writer, kept []config.PathEntry) {
	if len(kept) == 0 {
		return
	}
	fmt.Fprintln(out, "keeping user-scoped state (shared across projects); pass --global to remove:")
	for _, e := range kept {
		fmt.Fprintf(out, "  %s  (%s)\n", e.Path, e.Description)
	}
}

// Validate satisfies the cli.Command contract; nuke has nothing to validate.
func (c *NukeCommand) Validate(_ map[string]any) error { return nil }

// Help returns the one-line command summary.
func (c *NukeCommand) Help() string {
	return "Remove all blink state and config so the next run starts from scratch."
}
