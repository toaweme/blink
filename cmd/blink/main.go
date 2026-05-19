// Command blink is a local dev supervisor: it boots every service in
// blink.yaml, keeps them alive, and restarts them (and their declared
// dependents) on file changes, with a bubbletea TUI. It runs fully
// offline - no session sharing, no control socket, no remote access.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/toaweme/cli"
	"github.com/toaweme/cli/commands/help"
	"github.com/toaweme/log"

	"github.com/toaweme/blink/addons/docker"
	"github.com/toaweme/blink/addons/golang"
	"github.com/toaweme/blink/addons/portkill"
	"github.com/toaweme/blink/addons/shell"
	"github.com/toaweme/blink/blink/app"
	"github.com/toaweme/blink/core/addon"
)

// version, commit, and date are injected at build time by goreleaser via
// -ldflags -X main.<name>=...
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if err := run(); err != nil {
		log.Error("app failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	reg := addon.NewRegistry()
	reg.AddRuntime(
		shell.Runtime{},
		golang.Runtime{},
		docker.Runtime{},
	)
	reg.AddHook(portkill.Hook{})

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to resolve working directory: %w", err)
	}

	// load <cwd>/.env before config resolution; shell-set vars still win.
	if err := app.LoadDotEnv(cwd); err != nil {
		return fmt.Errorf("failed to load .env: %w", err)
	}

	cliApp := cli.NewApp(cli.Config{Name: "blink", Version: version}, cli.GlobalFlags{Cwd: cwd})
	cliApp.Add("help", help.NewHelpCommand(cliApp.Config, cliApp.Commands, cliApp.OutputFormats))

	commandRun := app.NewRunCommand(reg)

	cliApp.Add("run", commandRun)
	cliApp.Add("init", app.NewInitCommand())
	cliApp.Add("edit", app.NewEditCommand())
	cliApp.Default(commandRun)

	if err := cliApp.Run(os.Args[1:]); err != nil {
		if errors.Is(err, cli.ErrShowingHelp) {
			return nil
		}
		return fmt.Errorf("failed to run app: %w", err)
	}
	return nil
}
