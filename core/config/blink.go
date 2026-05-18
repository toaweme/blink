package config

import (
	"os"
	"path/filepath"
)

// Paths declares every directory and file blink reads or writes. All paths
// support env overrides. Relative paths resolve against Config.DirRoot (for
// project-scoped paths) or $HOME (for user-scoped paths). The loader fills
// defaults via Paths.Resolve() after loading blink.yaml.
//
// Convention: every path blink touches must live here. No os.Getenv for path
// discovery anywhere else in the codebase.
type Paths struct {
	// ConfigHome is the user-scoped config directory (~/.blink by default).
	// Holds auth tokens, nag state, CLI preferences. Override: $BLINK_CONFIG_HOME.
	ConfigHome string `toml:"config_home,omitempty" json:"config_home,omitempty" yaml:"config_home,omitempty" env:"BLINK_CONFIG_HOME"`
	// ControlDir is the project-scoped directory for the unix control socket
	// and any other per-project runtime state. Default: <DirRoot>/.blink.
	// Override: $BLINK_CONTROL_DIR.
	ControlDir string `toml:"control_dir,omitempty" json:"control_dir,omitempty" yaml:"control_dir,omitempty" env:"BLINK_CONTROL_DIR"`
	// LogDir is where headless-mode .log files are written for agent consumption.
	// Default: <DirRoot>/.blink/logs. Override: $BLINK_LOG_DIR.
	LogDir string `toml:"log_dir,omitempty" json:"log_dir,omitempty" yaml:"log_dir,omitempty" env:"BLINK_LOG_DIR"`
}

// Resolve fills any empty fields with platform defaults. Call after loading
// the config and before anything reads a path.
func (p *Paths) Resolve(dirRoot string) {
	home, _ := os.UserHomeDir()

	if p.ConfigHome == "" {
		if v := os.Getenv("BLINK_CONFIG_HOME"); v != "" {
			p.ConfigHome = v
		} else if home != "" {
			p.ConfigHome = filepath.Join(home, ".blink")
		}
	}

	// ControlDir / LogDir are populated from their env: tags by the cli
	// layer before Resolve runs, so Resolve only fills the default and
	// resolves a relative override against dirRoot - no os.Getenv here.
	if p.ControlDir == "" {
		p.ControlDir = filepath.Join(dirRoot, ".blink")
	} else if !filepath.IsAbs(p.ControlDir) {
		p.ControlDir = filepath.Join(dirRoot, p.ControlDir)
	}

	if p.LogDir == "" {
		p.LogDir = filepath.Join(dirRoot, ".blink", "logs")
	} else if !filepath.IsAbs(p.LogDir) {
		p.LogDir = filepath.Join(dirRoot, p.LogDir)
	}
}

// All returns every directory path for enumeration (used by nuke).
func (p *Paths) All() []PathEntry {
	return []PathEntry{
		{Path: p.ConfigHome, Description: "user-scoped config"},
		{Path: p.ControlDir, Description: "project state dir"},
		{Path: p.LogDir, Description: "service log files"},
	}
}

// PathEntry is a path with a human-readable description.
type PathEntry struct {
	Path        string
	Description string
}

// Config is the top-level blink configuration loaded from blink.yaml.
type Config struct {
	// Paths declares every directory and file blink reads or writes.
	// Empty fields are filled with platform defaults by Paths.Resolve().
	Paths Paths `toml:"paths,omitempty" json:"paths,omitempty" yaml:"paths,omitempty"`
	// UI selects the user interface implementation: "blink" (default TUI),
	// "plain" (line-prefixed stdout), "iterm2" (stub).
	UI string `toml:"ui,omitempty" json:"ui,omitempty" yaml:"ui,omitempty"`
	// UIStrategy is reserved for future split/tab layout options.
	UIStrategy string `toml:"ui_strategy,omitempty" json:"ui_strategy,omitempty" yaml:"ui_strategy,omitempty"`
	// DirTemp is the directory where temporary build artifacts live; watchers
	// exclude it from change detection.
	DirTemp string `toml:"dir_temp,omitempty" json:"dir_temp,omitempty" yaml:"dir_temp,omitempty"`
	// DirRoot is the project root all service Dir/Include paths resolve against.
	DirRoot string `toml:"dir_root,omitempty" json:"dir_root,omitempty" yaml:"dir_root,omitempty"`
	// Services is the ordered list of services blink supervises.
	Services []Service `toml:"services,omitempty" json:"services,omitempty" yaml:"services,omitempty"`
	// Zen starts the TUI in zen mode (no chrome, native scrollback).
	// Set from the CLI via `blink -z` / --zen / $BLINK_ZEN.
	Zen bool `toml:"zen,omitempty" json:"zen,omitempty" yaml:"zen,omitempty"`
	// ForceShutdown is the project-wide default for Service.ForceShutdown.
	// When true (the default when this field is nil), every service whose own
	// ForceShutdown isn't set explicitly will have its declared Ports scanned
	// and any owning process killed before the service starts. Override on a
	// per-service basis with Service.ForceShutdown.
	ForceShutdown *bool `toml:"force_shutdown,omitempty" json:"force_shutdown,omitempty" yaml:"force_shutdown,omitempty"`
	// Control configures the local shell-proxy. Disabled (zero value) by
	// default - `blink run` behaves exactly as before. When enabled, a Unix
	// socket lets other processes on the machine send stdin or signals to
	// supervised services (see `blink exec <target> stdin` / `blink exec <target> signal`).
	Control Control `toml:"control,omitempty" json:"control,omitempty" yaml:"control,omitempty"`
	// Logs configures per-service log-file writing. Independent of the UI:
	// any mode (blink TUI, plain, headless) writes <LogDir>/<svc>.log while
	// log writing is enabled. Flags (--logs/--no-logs) and the TUI `L` toggle
	// override this default at runtime.
	Logs LogConfig `toml:"logs,omitempty" json:"logs,omitempty" yaml:"logs,omitempty"`
	// ConfigPath is the absolute path the config was loaded from. Populated
	// by the loader at runtime and used by features that need to write the
	// config back. Never serialized.
	ConfigPath string `toml:"-" json:"-" yaml:"-"`
	// Runtime carries transient per-run options resolved from CLI flags
	// (e.g. --services). Never serialized.
	Runtime RuntimeOptions `toml:"-" json:"-" yaml:"-"`
}

// RuntimeOptions are CLI-only knobs that shape a single `blink run`
// invocation. Distinct from yaml-level config because they are not
// part of the project definition - they describe how to run it this
// time. Populated by command_run from --services, consumed by ui/blink
// and ui/plain.
type RuntimeOptions struct {
	// Services restricts which services start, by name. Empty = all.
	Services []string
}

// LogConfig controls per-service log-file writing.
type LogConfig struct {
	// Write toggles writing each service's captured output to
	// <LogDir>/<svc>.log. Nil defaults to true (logs are written unless
	// explicitly disabled). Set false (or --no-logs) to run the supervisor
	// without producing any log files.
	Write *bool `toml:"write,omitempty" json:"write,omitempty" yaml:"write,omitempty"`
}

// LogWriteEnabled reports whether per-service log files should be written.
// Defaults to true when unset.
func (c Config) LogWriteEnabled() bool {
	if c.Logs.Write != nil {
		return *c.Logs.Write
	}
	return true
}

// Control configures local TUI behaviour.
type Control struct {
	// Keys rebinds TUI keys onto the closed action catalog. Each entry is
	// key (bubbletea form, e.g. "r", "ctrl+c", "left") -> action name (see
	// control.Actions()). An empty value unbinds the key. Validated at load
	// time; an unknown action name is a hard error. Example:
	//   control:
	//     keys: { r: restart, R: restart-all, q: quit, z: toggle-zen }
	Keys map[string]string `toml:"keys,omitempty" json:"keys,omitempty" yaml:"keys,omitempty"`
}

// Service is a single supervised unit (a long-running process or a one-shot).
type Service struct {
	Name string `toml:"name,omitempty" json:"name,omitempty" yaml:"name,omitempty"`
	// Dir is the service directory relative to Config.DirRoot.
	Dir string `toml:"dir,omitempty" json:"dir,omitempty" yaml:"dir,omitempty"`
	// Runtime selects the lifecycle owner for this service. Empty = "shell"
	// (the default; runs the configured Commands as `sh -c`). Other value:
	// "go". A runtime contributes defaults that are merged into the
	// rest of this struct - anything the user sets explicitly wins.
	Runtime string `toml:"runtime,omitempty" json:"runtime,omitempty" yaml:"runtime,omitempty"`
	// Go holds the typed config for `runtime: go` services.
	Go       *GoConfig `toml:"go,omitempty" json:"go,omitempty" yaml:"go,omitempty"`
	Commands Commands  `toml:"commands,omitempty" json:"commands,omitempty" yaml:"commands,omitempty"`
	// Fs configures which file changes trigger restarts for this service.
	Fs Fs `toml:"fs,omitempty" json:"fs,omitempty" yaml:"fs,omitempty"`
	// Reload describes restart behavior and cross-service dependencies.
	Reload Reload            `toml:"reload,omitempty" json:"reload,omitempty" yaml:"reload,omitempty"`
	Env    map[string]string `toml:"env,omitempty" json:"env,omitempty" yaml:"env,omitempty"`
	// Logging configures per-service log handling (v1 honors Level only).
	Logging Logging `toml:"logging,omitempty" json:"logging,omitempty" yaml:"logging,omitempty"`
	// Ports lists TCP ports this service binds. blink scans them before start
	// (when ForceShutdown is on) and kills any process already listening so a
	// previous hanging child doesn't break the next run. This is project drift
	// (the dev server is what really chooses the port), but it makes the
	// developer experience reliable across crashes.
	Ports []int `toml:"ports,omitempty" json:"ports,omitempty" yaml:"ports,omitempty"`
	// ForceShutdown overrides Config.ForceShutdown for this service. Nil =
	// inherit. true = scan Ports and kill any listener before start. false =
	// never kill, even if the project-wide setting is on.
	ForceShutdown *bool `toml:"force_shutdown,omitempty" json:"force_shutdown,omitempty" yaml:"force_shutdown,omitempty"`
}

// GoConfig configures a `runtime: go` service. The runtime synthesizes build
// and run commands and auto-watches `go.work` module roots.
type GoConfig struct {
	// Package is the Go package path to build (e.g. "./cmd/v2/schema").
	Package string `toml:"package,omitempty" json:"package,omitempty" yaml:"package,omitempty"`
	// Args are passed to the built binary on `Run`.
	Args []string `toml:"args,omitempty" json:"args,omitempty" yaml:"args,omitempty"`
	// Out is the build output path. Default: "./build/<service-name>".
	Out string `toml:"out,omitempty" json:"out,omitempty" yaml:"out,omitempty"`
	// Workspace toggles go.work workspace watching. Default: true when a
	// go.work file is found alongside the service.
	Workspace *bool `toml:"workspace,omitempty" json:"workspace,omitempty" yaml:"workspace,omitempty"`
}

// Command is a shell command with optional Before/After chains.
type Command struct {
	Command        string    `toml:"command,omitempty" json:"command,omitempty" yaml:"command,omitempty"`
	CommandCleanup string    `toml:"command_cleanup,omitempty" json:"command_cleanup,omitempty" yaml:"command_cleanup,omitempty"`
	Dir            string    `toml:"dir,omitempty" json:"dir,omitempty" yaml:"dir,omitempty"`
	Before         []Command `toml:"before,omitempty" json:"before,omitempty" yaml:"before,omitempty"`
	After          []Command `toml:"after,omitempty" json:"after,omitempty" yaml:"after,omitempty"`
	// Service marks Command as a long-running process. The supervisor keeps it
	// alive and restarts it on file change; one-shot commands run to completion.
	Service bool `toml:"service,omitempty" json:"service,omitempty" yaml:"service,omitempty"`
}

type Commands struct {
	Build *Command `toml:"build,omitempty" json:"build,omitempty" yaml:"build,omitempty"`
	Run   *Command `toml:"run,omitempty" json:"run,omitempty" yaml:"run,omitempty"`
}

// Fs configures the file change watcher for a service.
//
// Include accepts paths (resolved against DirRoot) - they are added as recursive
// watch roots, enabling cross-module watches like "../schema". Extensions filter
// matched files; Exclude globs subtract from the matched set.
type Fs struct {
	Extensions []string `toml:"extensions,omitempty" json:"extensions,omitempty" yaml:"extensions,omitempty"`
	Include    []string `toml:"include,omitempty" json:"include,omitempty" yaml:"include,omitempty"`
	Exclude    []string `toml:"exclude,omitempty" json:"exclude,omitempty" yaml:"exclude,omitempty"`
}

// DefaultExcludeDirs are directory names blink subtracts from every watcher
// regardless of service config, matched at any depth. They're the single
// source the watcher's default globs and config detection both build on, so a
// detected service never re-emits an exclude blink already applies.
var DefaultExcludeDirs = []string{".git", "node_modules", "dist", "build", ".next", ".idea", ".vscode"}

// DefaultExcludes returns the glob patterns blink applies to every watcher:
// each DefaultExcludeDirs entry at any depth, plus .DS_Store files.
func DefaultExcludes() []string {
	out := make([]string, 0, len(DefaultExcludeDirs)+1)
	for _, d := range DefaultExcludeDirs {
		out = append(out, "**/"+d+"/**")
	}
	return append(out, "**/.DS_Store")
}

// Reload configures restart triggers.
type Reload struct {
	// Reload enables file-change-driven restart for this service.
	Reload bool `toml:"reload,omitempty" json:"reload,omitempty" yaml:"reload,omitempty"`
	// ReloadOnDelete restarts when a matching file is removed (e.g. an sqlite
	// db file). Air can't do this; blink can.
	ReloadOnDelete []string `toml:"reload_on_delete,omitempty" json:"reload_on_delete,omitempty" yaml:"reload_on_delete,omitempty"`
	// ReloadOnService cascades a restart whenever any listed service restarts.
	// Also encodes startup ordering: this service starts after its deps.
	ReloadOnService []string `toml:"reload_on_service,omitempty" json:"reload_on_service,omitempty" yaml:"reload_on_service,omitempty"`
}

// Logging configures per-service log handling. v1 only honors Level.
type Logging struct {
	// Level is "trace", "debug", "info", "warn", "error" or "fatal". Empty = inherit.
	Level string `toml:"level,omitempty" json:"level,omitempty" yaml:"level,omitempty"`
}
