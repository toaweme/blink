// Package docker is the docker-compose runtime. It runs the compose stack detached, watches container state via `docker events`, and streams every container's logs into the UI by default.
package docker

import (
	"os"
	"path/filepath"
	"strconv"

	"github.com/toaweme/blink/core/addon"
	"github.com/toaweme/blink/core/config"
)

// Runtime is the docker-compose runtime that runs services as a compose stack.
type Runtime struct{}

var _ addon.Runtime = Runtime{}

// Name returns the runtime identifier.
func (Runtime) Name() string { return "docker" }

// composeCandidates are the conventional compose filenames in preference order,
// mirroring the detector's list so a hand-written `runtime: docker` with no
// `file` resolves the same compose file `blink init` would have picked. The
// detector's copy lives in the unexported detect package, so the list is
// duplicated here to avoid an import cycle.
var composeCandidates = []string{"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml"}

// resolveComposeFile probes workDir for the conventional compose filenames in
// preference order and returns the first that exists, falling back to
// DefaultComposeFile when none are present.
func resolveComposeFile(workDir string) string {
	for _, name := range composeCandidates {
		if _, err := os.Stat(filepath.Join(workDir, name)); err == nil {
			return name
		}
	}
	return config.DefaultComposeFile
}

// Prepare builds the addon plan that drives the compose stack for the service.
func (r Runtime) Prepare(cfg config.Config, svc config.Service) (addon.Plan, error) {
	dc := svc.Docker
	if dc == nil {
		dc = &config.DockerConfig{}
	}

	workDir := filepath.Join(cfg.DirRoot, svc.Dir)

	composeFile := dc.File
	if composeFile == "" {
		composeFile = resolveComposeFile(workDir)
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

	// resolve the attach backlog to a docker `--tail` value: a bounded default,
	// an explicit count, or "all" (a non-positive config value) for full history.
	tail := strconv.Itoa(config.DefaultDockerLogTail)
	if dc.LogTail != nil {
		if *dc.LogTail <= 0 {
			tail = "all"
		} else {
			tail = strconv.Itoa(*dc.LogTail)
		}
	}

	mgr := newManager(managerOpts{
		Project:     proj,
		ComposeFile: composeFile,
		WorkDir:     workDir,
		Services:    append([]string(nil), dc.Services...),
		LogFilter:   append([]string(nil), dc.Logs...),
		Wait:        wait,
		StopOnExit:  dc.StopOnExit,
		LogTail:     tail,
	})

	return addon.Plan{
		// docker is managed end-to-end by the compose manager, so file-change reload defaults off.
		Defaults: config.Service{Reload: config.Reload{Reload: false}},
		Manager:  mgr,
	}, nil
}
