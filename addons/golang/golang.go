// Package golang is the Go-service runtime. Given a `runtime: go` service with
// a `go.package`, it synthesizes a `go build` + binary run command pair and -
// when the surrounding repo uses a workspace - adds every `go.work` `use`
// directory as a recursive watch root.
package golang

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/toaweme/blink/core/addon"
	"github.com/toaweme/blink/core/config"
)

type Runtime struct{}

var _ addon.Runtime = Runtime{}

func (Runtime) Name() string { return "go" }

func (r Runtime) Prepare(cfg config.Config, svc config.Service) (addon.Plan, error) {
	gc := svc.Go
	if gc == nil {
		return addon.Plan{}, fmt.Errorf("service %q: runtime: go requires a `go:` block", svc.Name)
	}
	if gc.Package == "" {
		return addon.Plan{}, fmt.Errorf("service %q: go.package is required", svc.Name)
	}

	out := gc.Out
	if out == "" {
		out = "./build/" + svc.Name
	}

	build := &config.Command{Command: fmt.Sprintf("go build -o %s %s", shellEscape(out), shellEscape(gc.Package))}
	run := &config.Command{Command: strings.TrimSpace(shellEscape(out) + " " + joinArgs(gc.Args)), Service: true}

	plan := addon.Plan{
		Defaults: config.Service{
			Fs:       config.Fs{Extensions: []string{"go", "mod", "sum"}},
			Commands: config.Commands{Build: build, Run: run},
		},
	}

	if workspaceEnabled(gc, cfg, svc) {
		watches, err := workspaceWatches(cfg, svc)
		if err != nil {
			return addon.Plan{}, fmt.Errorf("failed to resolve workspace watches for service %q: %w", svc.Name, err)
		}
		plan.ExtraWatches = watches
	}

	return plan, nil
}

func workspaceEnabled(gc *config.GoConfig, cfg config.Config, svc config.Service) bool {
	if gc.Workspace != nil {
		return *gc.Workspace
	}
	_, ok := findWorkfile(cfg, svc)
	return ok
}

// findWorkfile checks the service dir and DirRoot for go.work.
func findWorkfile(cfg config.Config, svc config.Service) (string, bool) {
	svcDir, err := filepath.Abs(filepath.Join(cfg.DirRoot, svc.Dir))
	if err != nil {
		return "", false
	}
	rootDir, err := filepath.Abs(cfg.DirRoot)
	if err != nil {
		return "", false
	}

	// check service dir first
	candidate := filepath.Join(svcDir, "go.work")
	if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
		return candidate, true
	}
	// check DirRoot if different from service dir
	if svcDir != rootDir {
		candidate = filepath.Join(rootDir, "go.work")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, true
		}
	}
	return "", false
}

// workspaceWatches returns absolute paths for every `use` directive in the
// go.work file that owns the service. The service's own directory is excluded
// (already a watch root via Service.Dir).
func workspaceWatches(cfg config.Config, svc config.Service) ([]string, error) {
	workfile, ok := findWorkfile(cfg, svc)
	if !ok {
		return nil, nil
	}
	uses, err := readWorkUses(workfile)
	if err != nil {
		return nil, err
	}

	workdir := filepath.Dir(workfile)
	svcAbs, err := filepath.Abs(filepath.Join(cfg.DirRoot, svc.Dir))
	if err != nil {
		return nil, err
	}

	var out []string
	for _, u := range uses {
		path := u
		if !filepath.IsAbs(path) {
			path = filepath.Join(workdir, path)
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			continue
		}
		if abs == svcAbs {
			continue
		}
		out = append(out, abs)
	}
	return out, nil
}

func shellEscape(s string) string {
	if s == "" {
		return "''"
	}
	if strings.ContainsAny(s, " \t\n'\"\\$`") {
		return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
	}
	return s
}

func joinArgs(args []string) string {
	parts := make([]string, 0, len(args))
	for _, a := range args {
		parts = append(parts, shellEscape(a))
	}
	return strings.Join(parts, " ")
}
