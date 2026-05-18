// Package shell is the no-op default runtime. It contributes no overlay
// and no manager - the supervisor just runs whatever the user wrote under
// `commands:`. Registered explicitly by the CLI so there is no init-time
// magic.
package shell

import (
	"github.com/toaweme/blink/core/addon"
	"github.com/toaweme/blink/core/config"
)

type Runtime struct{}

var _ addon.Runtime = Runtime{}

func (Runtime) Name() string { return "shell" }

func (Runtime) Prepare(_ config.Config, _ config.Service) (addon.Plan, error) {
	return addon.Plan{}, nil
}
