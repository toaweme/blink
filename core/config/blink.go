// Package config defines blink's configuration model and the paths blink reads and writes.
package config

import (
	"os"
	"path/filepath"
)

// Paths declares every directory and file blink reads or writes. ControlDir is
// the per-project root the other per-project paths derive from; their default
// names live in the consts below. A value set in the config file wins, then the
// $BLINK_<NAME> env override, then the default. Relative project paths resolve
// against DirRoot; ConfigHome is user-scoped and resolves against $HOME.
type Paths struct {
	// ConfigHome is the user-scoped config directory (auth tokens, nag state,
	// CLI preferences). Default: <blinkDirName> under $HOME. Env: $BLINK_CONFIG_HOME.
	ConfigHome string `yaml:"config_home,omitempty" json:"config_home,omitempty" toml:"config_home,omitempty"`
	// ControlDir is the per-project root for everything blink writes under the
	// project: logs, build output, and (later) the unix control socket. It is
	// the one directory the other per-project paths derive from. Default:
	// <blinkDirName> under DirRoot. Env: $BLINK_CONTROL_DIR.
	ControlDir string `yaml:"control_dir,omitempty" json:"control_dir,omitempty" toml:"control_dir,omitempty"`
	// LogDir is where per-service .log files are written. Default:
	// <ControlDir>/<logSubdir>. Env: $BLINK_LOG_DIR.
	LogDir string `yaml:"log_dir,omitempty" json:"log_dir,omitempty" toml:"log_dir,omitempty"`
	// BuildDir is where the go runtime writes compiled binaries, kept under the
	// control dir so build output shares the project's one artifact root.
	// Default: <ControlDir>/<buildSubdir>. Env: $BLINK_BUILD_DIR.
	BuildDir string `yaml:"build_dir,omitempty" json:"build_dir,omitempty" toml:"build_dir,omitempty"`
}

const (
	// blinkDirName is the directory blink keeps its state in, both user-scoped
	// (~/.blink) and per-project (<DirRoot>/.blink). Named once here so the
	// literal never appears scattered across the codebase.
	blinkDirName = ".blink"
	// logSubdir and buildSubdir live under the control dir, so every per-project
	// artifact shares one root.
	logSubdir   = "logs"
	buildSubdir = "build"
	// envPrefix is glued onto each field's NAME to form its override variable,
	// so the prefix is written once rather than baked into every field.
	envPrefix = "BLINK_"
)

// Resolve fills any empty field from its $BLINK_* override or default, deriving
// LogDir and BuildDir from ControlDir. Call after loading the config and before
// anything reads a path.
func (p *Paths) Resolve(dirRoot string) {
	if p.ConfigHome == "" {
		p.ConfigHome = os.Getenv(envPrefix + "CONFIG_HOME")
	}
	if p.ConfigHome == "" {
		if home, _ := os.UserHomeDir(); home != "" {
			p.ConfigHome = filepath.Join(home, blinkDirName)
		}
	}

	p.ControlDir = resolveDir(p.ControlDir, "CONTROL_DIR", dirRoot, filepath.Join(dirRoot, blinkDirName))
	p.LogDir = resolveDir(p.LogDir, "LOG_DIR", dirRoot, filepath.Join(p.ControlDir, logSubdir))
	p.BuildDir = resolveDir(p.BuildDir, "BUILD_DIR", dirRoot, filepath.Join(p.ControlDir, buildSubdir))
}

// resolveDir picks the directory for one path field: the value already set
// (config file) wins, else the $BLINK_<name> override, else def. A relative
// value resolves against dirRoot; def is expected to be absolute already.
func resolveDir(current, name, dirRoot, def string) string {
	v := current
	if v == "" {
		v = os.Getenv(envPrefix + name)
	}
	if v == "" {
		return def
	}
	if !filepath.IsAbs(v) {
		return filepath.Join(dirRoot, v)
	}
	return v
}

// All returns every directory path for enumeration (used by nuke).
func (p *Paths) All() []PathEntry {
	return []PathEntry{
		{Path: p.ConfigHome, Description: "user-scoped config", UserScoped: true},
		{Path: p.ControlDir, Description: "project state dir"},
		{Path: p.LogDir, Description: "service log files"},
		{Path: p.BuildDir, Description: "build output"},
	}
}

// PathEntry is a path with a human-readable description.
type PathEntry struct {
	Path        string
	Description string
	// UserScoped marks a path shared across every project (e.g. ~/.blink),
	// as opposed to project-scoped state under the current directory. nuke
	// removes user-scoped paths only when asked to wipe global state.
	UserScoped bool
}

// Config is the top-level blink configuration loaded from blink.yaml.
type Config struct {
	// Paths declares every directory and file blink reads or writes.
	// Empty fields are filled with defaults (and any $BLINK_* env override)
	// by Paths.Resolve().
	Paths Paths `yaml:"paths,omitempty" json:"paths,omitempty" toml:"paths,omitempty"`
	// UI selects the user interface implementation: "blink" (default TUI),
	// "plain" (line-prefixed stdout), "iterm2" (stub).
	UI string `yaml:"ui,omitempty" json:"ui,omitempty" toml:"ui,omitempty"`
	// UIStrategy is reserved for future split/tab layout options.
	UIStrategy string `yaml:"ui_strategy,omitempty" json:"ui_strategy,omitempty" toml:"ui_strategy,omitempty"`
	// DirRoot is the project root all service Dir/Include paths resolve against.
	DirRoot string `yaml:"dir_root,omitempty" json:"dir_root,omitempty" toml:"dir_root,omitempty"`
	// Services is the ordered list of services blink supervises.
	Services []Service `yaml:"services,omitempty" json:"services,omitempty" toml:"services,omitempty"`
	// Zen starts the TUI in zen mode (no chrome, native scrollback).
	// Set from the CLI via `blink -z` / --zen / $BLINK_ZEN.
	Zen bool `yaml:"zen,omitempty" json:"zen,omitempty" toml:"zen,omitempty"`
	// ForceShutdown is the project-wide default for Service.ForceShutdown.
	// When true (the default when nil), every service that does not set its own
	// ForceShutdown has its declared Ports scanned and any owning process killed
	// before start. Override per service with Service.ForceShutdown.
	ForceShutdown *bool `yaml:"force_shutdown,omitempty" json:"force_shutdown,omitempty" toml:"force_shutdown,omitempty"`
	// Control configures the local shell-proxy, disabled by default. When
	// enabled, a Unix socket lets other processes send stdin or signals to
	// supervised services.
	Control Control `yaml:"control,omitempty" json:"control,omitempty" toml:"control,omitempty"`
	// Logs configures per-service log-file writing, independent of the UI. While
	// enabled, every mode writes <LogDir>/<svc>.log. The --logs/--no-logs flags
	// and the TUI `L` toggle override this default at runtime.
	Logs LogConfig `yaml:"logs,omitempty" json:"logs,omitempty" toml:"logs,omitempty"`
	// ConfigPath is the absolute path the config was loaded from. Populated
	// by the loader at runtime and used by features that need to write the
	// config back. Never serialized.
	ConfigPath string `yaml:"-" json:"-" toml:"-"`
	// Runtime carries transient per-run options resolved from CLI flags
	// (e.g. --services). Never serialized.
	Runtime RuntimeOptions `yaml:"-" json:"-" toml:"-"`
}

// RuntimeOptions are CLI-only knobs shaping a single `blink run` invocation.
// They are not part of the project definition and are never serialized.
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
	Write *bool `yaml:"write,omitempty" json:"write,omitempty" toml:"write,omitempty"`
}

// LogWriteEnabled reports whether per-service log files should be written.
// Defaults to true when unset.
func (c Config) LogWriteEnabled() bool {
	if c.Logs.Write != nil {
		return *c.Logs.Write
	}
	return true
}

// Control configures local TUI behavior.
type Control struct {
	// Keys rebinds TUI keys onto the action catalog. Each entry maps a key
	// (bubbletea form, e.g. "r", "ctrl+c") to an action name (see
	// control.Actions()); an empty value unbinds the key. Validated at load
	// time: an unknown action name is a hard error. Example:
	//   control:
	//     keys: { r: restart, R: restart-all, q: quit, z: toggle-zen }
	Keys map[string]string `yaml:"keys,omitempty" json:"keys,omitempty" toml:"keys,omitempty"`
}

// Service is a single supervised unit (a long-running process or a one-shot).
type Service struct {
	Name string `yaml:"name,omitempty" json:"name,omitempty" toml:"name,omitempty"`
	// Dir is the service directory relative to Config.DirRoot.
	Dir string `yaml:"dir,omitempty" json:"dir,omitempty" toml:"dir,omitempty"`
	// Runtime selects the lifecycle owner for this service. Empty = "shell"
	// (the default; runs the configured Commands as `sh -c`). Other values:
	// "docker", "go", "node". A runtime contributes defaults that are merged
	// into the rest of this struct - anything the user sets explicitly wins.
	Runtime string `yaml:"runtime,omitempty" json:"runtime,omitempty" toml:"runtime,omitempty"`
	// Docker holds the typed config for `runtime: docker` services.
	Docker *DockerConfig `yaml:"docker,omitempty" json:"docker,omitempty" toml:"docker,omitempty"`
	// Node holds the typed config for `runtime: node` services.
	Node *NodeConfig `yaml:"node,omitempty" json:"node,omitempty" toml:"node,omitempty"`
	// Go holds the typed config for `runtime: go` services.
	Go       *GoConfig `yaml:"go,omitempty" json:"go,omitempty" toml:"go,omitempty"`
	Commands Commands  `yaml:"commands,omitempty" json:"commands,omitempty" toml:"commands,omitempty"`
	// Fs configures which file changes trigger restarts for this service.
	Fs Fs `yaml:"fs,omitempty" json:"fs,omitempty" toml:"fs,omitempty"`
	// Reload describes restart behavior and cross-service dependencies.
	Reload Reload            `yaml:"reload,omitempty" json:"reload,omitempty" toml:"reload,omitempty"`
	Env    map[string]string `yaml:"env,omitempty" json:"env,omitempty" toml:"env,omitempty"`
	// Logging configures per-service log handling
	Logging Logging `yaml:"logging,omitempty" json:"logging,omitempty" toml:"logging,omitempty"`
	// Ports lists TCP ports this service binds. When ForceShutdown is on, blink
	// scans them before start and kills any process already listening, so a
	// previous hanging child does not break the next run.
	Ports []Port `yaml:"ports,omitempty" json:"ports,omitempty" toml:"ports,omitempty"`
	// ForceShutdown overrides Config.ForceShutdown for this service. Nil =
	// inherit. true = scan Ports and kill any listener before start. false =
	// never kill, even if the project-wide setting is on.
	ForceShutdown *bool `yaml:"force_shutdown,omitempty" json:"force_shutdown,omitempty" toml:"force_shutdown,omitempty"`
}

// DefaultComposeFile is the compose file the docker runtime uses when
// DockerConfig.File is empty. Detection writes File only when it differs.
const DefaultComposeFile = "docker-compose.yml"

// DockerConfig configures a `runtime: docker` service. The runtime drives
// `docker compose` and exposes per-container status and logs to the supervisor.
type DockerConfig struct {
	// File is the compose file path relative to the service Dir (or DirRoot
	// if Dir is empty). Default: "docker-compose.yml".
	File string `yaml:"file,omitempty" json:"file,omitempty" toml:"file,omitempty"`
	// Project sets compose's `-p`. Default: basename of the resolved Dir.
	Project string `yaml:"project,omitempty" json:"project,omitempty" toml:"project,omitempty"`
	// Services restricts the compose stack to a subset. Empty = all services
	// defined in the compose file.
	Services []string `yaml:"services,omitempty" json:"services,omitempty" toml:"services,omitempty"`
	// Logs narrows which container logs blink streams into the UI. Empty
	// (default) streams every container in the running stack; set it to a
	// subset to follow only those.
	Logs []string `yaml:"logs,omitempty" json:"logs,omitempty" toml:"logs,omitempty"`
	// Wait toggles `docker compose up --wait`. Default: true.
	Wait *bool `yaml:"wait,omitempty" json:"wait,omitempty" toml:"wait,omitempty"`
	// StopOnExit makes blink stop the containers it started when it exits.
	// Default: false, so containers persist between runs and the next startup
	// reuses warm databases. Pre-existing containers are never touched.
	StopOnExit bool `yaml:"stop_on_exit,omitempty" json:"stop_on_exit,omitempty" toml:"stop_on_exit,omitempty"`
}

// IsZero reports whether every field is its zero value, so the writer drops an
// all-default docker block instead of serializing "docker: {}". yaml.v3 honors
// this (IsZeroer) for omitempty.
func (d DockerConfig) IsZero() bool {
	return d.File == "" && d.Project == "" && len(d.Services) == 0 &&
		len(d.Logs) == 0 && d.Wait == nil && !d.StopOnExit
}

// NodeConfig configures a `runtime: node` service. The runtime detects the
// package manager from the service's lockfile and synthesizes a
// `<pm> run <script>` command, optionally preceded by a guarded install.
type NodeConfig struct {
	// Script is the package.json script to run as `<pm> run <script>`.
	// Default: "dev".
	Script string `yaml:"script,omitempty" json:"script,omitempty" toml:"script,omitempty"`
	// PackageManager overrides the auto-detected manager ("npm", "pnpm",
	// "yarn", "bun"). Empty = detect from the lockfile.
	PackageManager string `yaml:"package_manager,omitempty" json:"package_manager,omitempty" toml:"package_manager,omitempty"`
	// Install toggles the dependency install step. Nil defaults to true.
	// When enabled the runtime emits `<pm> install` as a Setup command, which
	// runs once on boot and again only when package.json changes, never on an
	// ordinary source reload. Set false to skip installs entirely (e.g. to
	// manage them yourself via a custom commands.setup).
	Install *bool `yaml:"install,omitempty" json:"install,omitempty" toml:"install,omitempty"`
}

// IsZero reports whether every field is its zero value, so the writer drops an
// all-default node block instead of serializing "node: {}". yaml.v3 honors this
// (IsZeroer) for omitempty.
func (n NodeConfig) IsZero() bool {
	return n.Script == "" && n.PackageManager == "" && n.Install == nil
}

// GoConfig configures a `runtime: go` service. The runtime synthesizes build
// and run commands and auto-watches `go.work` module roots.
type GoConfig struct {
	// Package is the Go package path to build (e.g. "./cmd/v2/schema").
	Package string `yaml:"package,omitempty" json:"package,omitempty" toml:"package,omitempty"`
	// Args are passed to the built binary on `Run`.
	Args []string `yaml:"args,omitempty" json:"args,omitempty" toml:"args,omitempty"`
	// Out is the build output path. Default: <Paths.BuildDir>/<service-name>
	// (under the project's .blink dir). Set it to override the location.
	Out string `yaml:"out,omitempty" json:"out,omitempty" toml:"out,omitempty"`
	// Workspace toggles go.work workspace watching. Default: true when a
	// go.work file is found alongside the service.
	Workspace *bool `yaml:"workspace,omitempty" json:"workspace,omitempty" toml:"workspace,omitempty"`
}

// Command is a shell command with optional Before/After chains.
type Command struct {
	Command        string    `yaml:"command,omitempty" json:"command,omitempty" toml:"command,omitempty"`
	CommandCleanup string    `yaml:"command_cleanup,omitempty" json:"command_cleanup,omitempty" toml:"command_cleanup,omitempty"`
	Dir            string    `yaml:"dir,omitempty" json:"dir,omitempty" toml:"dir,omitempty"`
	Before         []Command `yaml:"before,omitempty" json:"before,omitempty" toml:"before,omitempty"`
	After          []Command `yaml:"after,omitempty" json:"after,omitempty" toml:"after,omitempty"`
	// Service marks Command as a long-running process. The supervisor keeps it
	// alive and restarts it on file change; one-shot commands run to completion.
	Service bool `yaml:"service,omitempty" json:"service,omitempty" toml:"service,omitempty"`
}

// Commands groups a service's optional build and run commands.
type Commands struct {
	// Setup runs once when the service first boots and again only when a
	// runtime-declared trigger file changes (e.g. package.json), never on an
	// ordinary source-file reload. It is where one-time preparation like
	// dependency installs belongs, so a live reload stays fast.
	Setup []Command `yaml:"setup,omitempty" json:"setup,omitempty" toml:"setup,omitempty"`
	Build *Command  `yaml:"build,omitempty" json:"build,omitempty" toml:"build,omitempty"`
	Run   *Command  `yaml:"run,omitempty" json:"run,omitempty" toml:"run,omitempty"`
}

// Fs configures the file change watcher for a service. Include paths (resolved
// against DirRoot) are added as recursive watch roots, enabling cross-module
// watches like "../schema". Extensions filter matched files; Exclude globs
// subtract from the matched set.
type Fs struct {
	Extensions []string `yaml:"extensions,omitempty" json:"extensions,omitempty" toml:"extensions,omitempty"`
	Include    []string `yaml:"include,omitempty" json:"include,omitempty" toml:"include,omitempty"`
	Exclude    []string `yaml:"exclude,omitempty" json:"exclude,omitempty" toml:"exclude,omitempty"`
}

// DefaultExcludeDirs are directory names blink subtracts from every watcher at
// any depth, regardless of service config. The watcher's default globs and
// config detection both build on this, so a detected service never re-emits an
// exclude blink already applies.
var DefaultExcludeDirs = []string{".git", "node_modules", "dist", "build", ".next", ".idea", ".vscode", blinkDirName}

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
	Reload bool `yaml:"reload,omitempty" json:"reload,omitempty" toml:"reload,omitempty"`
	// ReloadOnDelete restarts when a matching file is removed (e.g. an sqlite
	// db file). Air can't do this; blink can.
	ReloadOnDelete []string `yaml:"reload_on_delete,omitempty" json:"reload_on_delete,omitempty" toml:"reload_on_delete,omitempty"`
	// ReloadOnService cascades a restart whenever any listed service restarts.
	// Also encodes startup ordering: this service starts after its deps.
	ReloadOnService []string `yaml:"reload_on_service,omitempty" json:"reload_on_service,omitempty" toml:"reload_on_service,omitempty"`
}

// Logging configures per-service log handling.
type Logging struct {
	// Level is "trace", "debug", "info", "warn", "error" or "fatal". Empty = inherit.
	Level string `yaml:"level,omitempty" json:"level,omitempty" toml:"level,omitempty"`
}
