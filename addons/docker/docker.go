// Package docker is the docker-compose runtime. It runs the configured
// compose stack detached, watches container state via `docker events` (no
// polling), and streams every container's logs into the UI by default.
package docker

import (
	"path/filepath"

	"github.com/toaweme/blink/core/addon"
	"github.com/toaweme/blink/core/config"
)

type Runtime struct{}

var _ addon.Runtime = Runtime{}

func (Runtime) Name() string { return "docker" }

func (r Runtime) Prepare(cfg config.Config, svc config.Service) (addon.Plan, error) {
	dc := svc.Docker
	if dc == nil {
		dc = &config.DockerConfig{}
	}

	workDir := filepath.Join(cfg.DirRoot, svc.Dir)

	composeFile := dc.File
	if composeFile == "" {
		composeFile = config.DefaultComposeFile
	}
	if !filepath.IsAbs(composeFile) {
		composeFile = filepath.Join(workDir, composeFile)
	}

	proj := dc.Project
	if proj == "" {
		proj = filepath.Base(workDir)
	}

	wait := true
	if dc.Wait != nil {
		wait = *dc.Wait
	}

	mgr := newManager(managerOpts{
		Project:     proj,
		ComposeFile: composeFile,
		WorkDir:     workDir,
		Services:    append([]string(nil), dc.Services...),
		LogFilter:   append([]string(nil), dc.Logs...),
		Wait:        wait,
		StopOnExit:  dc.StopOnExit,
	})

	return addon.Plan{
		// Docker doesn't restart on file changes - it's managed end-to-end by
		// the compose manager. Watchers default off here.
		Defaults: config.Service{Reload: config.Reload{Reload: false}},
		Manager:  mgr,
	}, nil
}
