// Package shell is the no-op default runtime. It contributes no overlay and no manager: the supervisor runs whatever the user wrote under `commands:`. Registered explicitly by the CLI.
package shell

import (
	"github.com/toaweme/blink/core/addon"
	"github.com/toaweme/blink/core/config"
)

// Runtime is the no-op default runtime that runs the user's own commands.
type Runtime struct{}

var _ addon.Runtime = Runtime{}

// Name returns the runtime identifier.
func (Runtime) Name() string { return "shell" }

// Prepare returns an empty plan; the shell runtime contributes no overlay.
func (Runtime) Prepare(_ config.Config, _ config.Service) (addon.Plan, error) {
	return addon.Plan{}, nil
}
