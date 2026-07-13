# Configuring blink

`blink init` generates a working `blink.yml` and zero-config `blink run` needs no file at all, so most projects never edit configuration by hand.
This document is the complete reference for the cases that do, whether you are tuning a service manually, understanding a generated field, or overriding where blink stores its state.

The [Quick reference](#quick-reference) covers the common case.
The sections that follow document every field.

## Contents

- [Quick reference](#quick-reference)
- [Discovery and loading](#discovery-and-loading)
- [Top-level fields](#top-level-fields)
- [Paths](#paths)
- [Environment variables](#environment-variables)
- [Services](#services)
  - [Generic fields](#generic-service-fields)
  - [`commands`](#commands)
  - [`fs`](#fs)
  - [`reload`](#reload)
  - [`ports` and `force_shutdown`](#ports-and-force_shutdown)
- [Runtimes](#runtimes)
  - [`shell`](#runtime-shell-default)
  - [`go`](#runtime-go)
  - [`node`](#runtime-node)
  - [`docker`](#runtime-docker)
- [TUI keybindings](#tui-keybindings)

## Quick reference

A representative `blink.yml`. Every field shown here is optional unless noted.

```yaml
# top-level (all optional)
ui: blink                 # blink (TUI) | plain (line-prefixed stdout) | headless
force_shutdown: true      # kill anything on a service's declared ports before start
logs:
  write: true             # write <control_dir>/logs/<svc>.log per service

services:
  - name: db
    runtime: docker       # drives `docker compose`, streams every container's logs

  - name: api
    runtime: go           # builds and runs a Go package, auto-watches go.work roots
    go:
      package: ./cmd/api
    ports: [8080]         # reclaimed before start when force_shutdown is on
    reload:
      reload: true        # restart on file change
      reload_on_service: [db]   # start after db, and restart whenever db restarts

  - name: web
    runtime: node         # detects the package manager, runs `<pm> run dev`
    dir: ../ui            # services can live in sibling repos
    node:
      script: dev

  - name: worker
    runtime: shell        # the default, runs `commands` as `sh -c`
    commands:
      run:
        command: ./worker
        service: true     # a long-running process, not a one-shot
    reload:
      reload: true
    fs:
      extensions: [go]

control:
  keys:                   # TUI key rebinds (see TUI keybindings)
    r: restart
    q: quit
```

## Discovery and loading

blink reads one project config file.
With no explicit path it walks up from the current working directory and takes the first file it finds, so you can run blink from any subdirectory of the project.

The names searched, in order, are `blink.yml`, `blink.yaml`, `blink.toml`, `blink.json`.
`blink.yml` is canonical (the name `blink init` writes) and the others are accepted fallbacks.
The format is chosen from the file extension, not the contents, and each maps to a codec built on a library already vendored (`encoding/json`, `gopkg.in/yaml.v3`, `pelletier/go-toml/v2`).

`-c` / `--config` (or `$BLINK_CONFIG`) names a config path explicitly.
When set, the walk-up is skipped and that path is used directly.
A named-but-missing config or a parse error fails hard.
Only a missing config with no `-c` falls back to zero-config detection.

**`.env` auto-load.** Before any config or `$BLINK_*` override is read, blink loads `<cwd>/.env` into the process environment.
It only sets keys that are not already present, so a value exported in your shell wins over the file.
A missing `.env` is not an error.
Because this runs first, any `$BLINK_*` variable can also be supplied through `.env`.
This is a single top-level `.env`; per-service `.env` files are read only during detection to sniff ports, never loaded into the environment at run time.

## Top-level fields

| Field | Type | Default | Meaning |
| --- | --- | --- | --- |
| `ui` | string | auto | interface backend. `blink` (TUI), `plain` (line-prefixed stdout), `headless` (alias `none`). Empty auto-selects `plain` when stdout is not a TTY, else `blink`. |
| `dir_root` | string | config file's directory | the base every service `dir`/`include` resolves against. |
| `zen` | bool | false | start the TUI without chrome (native scrollback). Honored from the config file, and `-z`/`--zen` forces it on, see the note below. |
| `force_shutdown` | bool | true | project-wide default for reclaiming a service's declared ports before start. Override per service. |
| `logs.write` | bool | true | write `<log_dir>/<svc>.log` for every service. The `--logs`/`--no-logs` flags and the TUI `L` key override at run time. |
| `paths` | object | derived | directory layout, see [Paths](#paths). |
| `control.keys` | map | default keymap | TUI key rebinds, see [TUI keybindings](#tui-keybindings). |
| `services` | list | (none) | the supervised units, see [Services](#services). |

An unknown `ui` value fails at run time.
`force_shutdown` and `logs.write` are nullable, so leaving them out is not the same as setting `false`; unset falls back to the default shown above.

> **Note on `zen`.** A `zen: true` in the config file is honored on its own, and `-z`/`--zen` forces zen on when the config leaves it off.
> The flag is a plain on-toggle, so it cannot turn zen back off when the config already sets `zen: true`; drop the config value for that.

## Paths

The `paths` block declares the four directories blink reads and writes.
Any field you leave empty is filled in this precedence order, config file value first, then the matching `$BLINK_*` env override, then the derived default.

| Field | Default | Scope | Meaning |
| --- | --- | --- | --- |
| `config_home` | `$HOME/.blink` | user | user-scoped state (auth, nag state, prefs). The only path `nuke --global` removes. |
| `control_dir` | `<dir_root>/.blink` | project | per-project state root the other project paths derive from. |
| `log_dir` | `<control_dir>/logs` | project | per-service `.log` files. |
| `build_dir` | `<control_dir>/build` | project | compiled Go binaries. |

State (logs, build output) lives under the per-project `.blink/` dir, and user-scoped config lives in `~/.blink`.
A relative override for `control_dir`, `log_dir`, or `build_dir` resolves against `dir_root`, while a relative `config_home` resolves against `$HOME` to match its user scope.
An absolute value is used verbatim in every case.

## Environment variables

blink reads a small set of `$BLINK_*` variables.
`BLINK_CONFIG` is the env form of the `-c` flag, and the four path variables relocate where blink stores state.
The path variables are the ones you reach for in CI or a container, where passing flags is awkward and there may be no config file.
Every other setting is a flag only; the run, init, and nuke commands do not read a `BLINK_*` mirror of their flags.

| Variable | Flag | Command | Effect | Default |
| --- | --- | --- | --- | --- |
| `BLINK_CONFIG` | `-c` / `--config` | run, init, edit | explicit config path (skips walk-up); on `init` names the file to create | walk up from cwd (init: `blink.yml`) |
| `BLINK_CONFIG_HOME` | (none) | paths | user-scoped state dir | `$HOME/.blink` |
| `BLINK_CONTROL_DIR` | (none) | paths | per-project state root | `<dir_root>/.blink` |
| `BLINK_LOG_DIR` | (none) | paths | per-service log dir | `<control_dir>/logs` |
| `BLINK_BUILD_DIR` | (none) | paths | Go build output dir | `<control_dir>/build` |

## Services

`services:` is the ordered list of units blink supervises.
The fields below apply to every runtime, and the typed `go` / `node` / `docker` blocks are documented under [Runtimes](#runtimes).

### Generic service fields

| Field | Type | Default | Meaning |
| --- | --- | --- | --- |
| `name` | string | (required) | unique identifier. Every reference (`reload_on_service`, log file, TUI row) keys off it. Empty or duplicate names are rejected. |
| `disabled` | bool | false | keep the service in the file but leave it out of the run. Never started, never shown. `blink edit` sets this when you deselect, so config survives. |
| `dir` | string | `dir_root` | service working directory, relative to `dir_root`. Commands run here. |
| `runtime` | string | `shell` | lifecycle owner. `shell`, `go`, `node`, `docker`. See [Runtimes](#runtimes). |
| `env` | map | (none) | string map merged into every command's environment, and the lookup source for env-referenced ports. Your keys win over runtime-contributed ones. |

A non-shell runtime contributes defaults (commands, watched extensions, install steps) that are merged into the service, and anything you set explicitly wins.

### `commands`

Three optional phases run in order on every boot or restart.

```yaml
    commands:
      setup:
        - command: go mod download
      build:
        command: go build -o ./bin/api ./cmd/api
      run:
        command: ./bin/api
        before:
          - command: ./scripts/migrate.sh
        after:
          - command: ./scripts/notify-down.sh
        command_cleanup: ./scripts/cleanup.sh
        service: true
```

`setup` (a list) runs once on the first boot and again only when a runtime-declared trigger file changes (a manifest or lockfile such as `package.json` or `go.mod`).
It never runs on an ordinary source reload, so a live reload stays fast.
This is where one-time preparation like dependency installs belongs.
The trigger set comes from the runtime and is not user-configurable.

`build` (a single command) runs on every boot and on every restart, before the run command.
Status shows `building` while it runs, and a failing build marks the service `crashed` and skips run.
A cached-build expectation does not hold here, since build re-runs on each restart.

`run` (a single command) is the main process.
Status shows `running` once started.

Each `run`/`build`/`before`/`after`/`setup` entry is a `Command`.

| Field | Type | Meaning |
| --- | --- | --- |
| `command` | string | the shell command. Empty is skipped. |
| `dir` | string | joined onto the service dir for this command only. |
| `before` | list | commands run to completion before `command`. |
| `after` | list | commands run after `command` (skipped for a long-running `service: true` process, see below). |
| `command_cleanup` | string | shell command run at supervisor shutdown. Honored only on the `run` command, and ignored if set on any other phase. |
| `service` | bool | marks the command as a long-running process rather than a one-shot. |

**One-shot vs `service: true`.**
A one-shot run command (the default) is expected to finish; when it completes its `after` chain runs and the status becomes `exited`.
A long-running command (`service: true`) is the service process; when it exits the status becomes `exited` and the `after` chain is skipped.
That skipped `after` chain is the only effect of `service: true`.
blink does not auto-respawn a service process on its own; a restart comes only from a file change (see `reload`) or a manual `r` in the TUI.

When a runtime contributes commands, its `setup`/`before`/`after` items are prepended before yours, so a runtime install runs ahead of your custom steps.

### `fs`

Controls which file changes the watcher reacts to.
Consulted only when reload is enabled (see [`reload`](#reload)).

```yaml
    fs:
      extensions: [go, sql]
      include:
        - ../schema
      exclude:
        - "**/testdata/**"
```

`extensions` filters matched files by extension (leading dot optional).
Empty watches every extension.
Matching is case-insensitive, so `Main.GO` matches a configured `go`.

`include` adds extra recursive watch roots, resolved against `dir_root`.
This is how you watch a cross-module path like `../schema` that lives outside the service dir.
When every `include` entry is a directory, those roots plus the implicit service-dir root are all watched.
When every `include` entry is a file, only those files trigger restarts and the implicit service-dir root is dropped, so a file-only `include` that still expects the service dir to be watched will miss its source edits.
A path that does not exist yet is watched through its parent directory.

`exclude` is a list of globs subtracted from the matched set, on top of the built-in excludes.

**Built-in excludes.**
blink always excludes `.git`, `node_modules`, `dist`, `build`, `.next`, `.idea`, `.vscode`, `.blink` at any depth, plus `.DS_Store`.
Note that a service whose real source lives in a directory literally named `build` or `dist` will not be watched there.

### `reload`

Restart triggers and cross-service dependencies.

```yaml
    reload:
      reload: true
      reload_on_delete:
        - "**/*.sqlite"
      reload_on_service:
        - db
```

`reload` enables file-change-driven restart.
Without it (and without `reload_on_delete`) the service gets no watcher and never restarts on edits.
Reload is not implied by the `runtime:` field, so a plain shell service must set `reload: true` explicitly.
When a service has no watcher because no reload is configured, blink logs an info-level hint at startup so the silence is not a surprise.
`blink init` writes `reload: true` for detected services, and the docker runtime deliberately leaves it false since compose manages its own containers.

`reload_on_delete` is a list of globs; a restart fires when a matching file is removed (for example an sqlite db file).
It works even with `reload: false`, in which case only deletes restart, not content changes.

`reload_on_service` does two things.
It orders startup, so this service waits for each listed dependency to reach `running` (or `exited`, for a one-shot) before it starts.
And it cascades restarts, so whenever a listed dependency restarts this service restarts too.
If a listed dependency crashes on boot, the dependent does not hang; it is marked `crashed` with a diagnostic naming the failed dependency.
Unknown dependency names and cycles are rejected at load time.

### `ports` and `force_shutdown`

`ports` lists the TCP ports the service binds.
They feed the port-reclaim step that runs just before a service starts.

```yaml
    ports:
      - 8080          # literal
      - PORT          # env-var name, resolved at runtime
      - ${API_PORT}   # sigil accepted on input, stored as API_PORT
    force_shutdown: false
```

A bare integer is a literal port.
Anything non-numeric is an env-var name; a `${KEY}` or `$KEY` sigil is stripped on input, and the reference resolves at runtime from the service `env` first, then the process environment.
A literal must be 1-65535, and a reference that does not resolve into that range is dropped from the scan with a logged warning so a typo'd name is visible.

`force_shutdown` on a service overrides the project-wide default and controls the reclaim step.
`nil` (unset) inherits the top-level `force_shutdown`, which itself defaults to `true`.
`true` scans this service's resolved ports and sends SIGTERM (then SIGKILL to any process still listening after 150ms), so a lingering child from a previous run cannot block the new bind; blink skips its own process and its parent.
`false` never reclaims, even when the project default is on.
The reclaim only does anything when the service also declares `ports`, and it is best-effort, so a missing `lsof` or a permission error is logged and never blocks the start.

## Runtimes

Every service has a `runtime` that names its lifecycle owner.
Empty means `shell`.
The other three read a typed block of the same name and synthesize commands, watch defaults, and managers.
Whatever a runtime contributes is a default, so anything you set explicitly wins.
Each runtime resolves the same defaults `blink init` writes, so a minimal hand-written `runtime:` block behaves the same as a generated one.

### `runtime: shell` (default)

The fallback.
When `runtime` is empty or `shell`, blink runs whatever you put under `commands:` as `sh -c`.
The shell runtime contributes no overlay, no watch defaults, and no manager, so a shell service that should live-reload needs an explicit `reload: true` and an `fs` block.

### `runtime: go`

Synthesizes a build plus binary-run pair and, when the repo uses a `go.work` workspace, adds every workspace module as an extra watch root.

```yaml
  - name: api
    runtime: go
    go:
      package: ./cmd/api      # required
      args: ["--port", "8080"]
      out: ./bin/api          # optional, defaults under .blink/build
      workspace: true         # optional, auto-detected from go.work
```

| Field | Type | Default | Meaning |
| --- | --- | --- | --- |
| `package` | string | (required) | Go package to build, e.g. `./cmd/api`. Empty is a hard error. |
| `args` | list | (none) | arguments passed to the built binary on run. |
| `out` | string | `<build_dir>/<name>` | build output path. `build_dir` itself is overridable (see [Paths](#paths)). |
| `workspace` | bool | true when `go.work` present | watch `go.work` module roots. |

Build is `go build -o <out> <package>`, run is `<out> <args...>` marked as a long-running service.
Default watched extensions are `go`, `mod`, `sum`, and `blink init` leaves the extension list unset so a generated go service watches these same three.
With a `go.work` present, blink parses its `use` directives and adds each module (except the service's own dir) as a recursive watch root, so a cross-module edit rebuilds.
Set `workspace: false` to opt out even when a `go.work` exists.

### `runtime: node`

Detects the package manager from the lockfile and synthesizes `<pm> run <script>`, optionally preceded by a guarded install.

```yaml
  - name: web
    runtime: node
    node:
      script: dev            # optional, defaults to dev then start
      package_manager: pnpm  # optional, auto-detected from lockfile
      install: true          # optional, defaults to true
```

| Field | Type | Default | Meaning |
| --- | --- | --- | --- |
| `script` | string | `dev`, else `start` | package.json script run as `<pm> run <script>`. |
| `package_manager` | string | detected | `npm`, `pnpm`, `yarn`, `bun`. Empty means detect, and an unknown value is rejected at load. |
| `install` | bool | true | when on, emits `<pm> install` as a setup step. |

When `script` is unset the runtime reads the service's `package.json` and picks `dev` if present, else `start`, the same order the detector uses, so a start-only project runs its `start` script rather than a failing `<pm> run dev`.
Package-manager detection order is pnpm, then bun, then yarn, then npm, keyed on the first lockfile present, falling back to npm when none is found.
Install is a setup command, so it runs once on boot and again only when `package.json` changes, never on a source reload, and lockfile rewrites never loop back into another install.

A self-reloading dev server (vite, next, nuxt, astro, remix, sveltekit, parcel, nodemon, `tsx watch`, `--watch`, and similar) is detected from the resolved dev command.
blink then assumes the tool handles HMR itself and scopes the watch to `include: [package.json]` only, so source edits never restart the process (a `package.json` change still reinstalls and restarts).
A plain node server keeps the broad `js/jsx/ts/tsx/mjs/cjs/json/css/html` watch, with lockfiles excluded.

### `runtime: docker`

Drives `docker compose`, watches container state via `docker events`, and by default streams every container's logs multiplexed into the UI.
File-change reload is off, because compose owns the containers.

```yaml
  - name: infra
    runtime: docker
    docker:
      file: docker-compose.yml   # optional
      project: myapp             # optional, defaults to basename of dir
      services: [db, redis]      # optional, empty = all in compose file
      logs: [db]                 # optional, empty = stream all containers
      wait: true                 # optional, defaults true
      stop_on_exit: false        # optional, defaults false
      log_tail: 500              # optional, defaults 500, <=0 = all
```

| Field | Type | Default | Meaning |
| --- | --- | --- | --- |
| `file` | string | probed (see below) | compose file relative to the service `dir` (or `dir_root`). |
| `project` | string | basename of resolved dir | compose `-p` project name. |
| `services` | list | all | restricts which compose services are brought up. |
| `logs` | list | all running | narrows which container logs stream into the UI. |
| `wait` | bool | true | toggles `docker compose up --wait`. |
| `stop_on_exit` | bool | false | stop blink-started containers on exit. |
| `log_tail` | int | 500 | recent lines each container replays on attach. `<=0` means all history. |

When `file` is unset the runtime probes `compose.yaml`, `compose.yml`, `docker-compose.yaml`, `docker-compose.yml` in that order (the same list the detector uses) and falls back to `docker-compose.yml` when none exist, so a `compose.yaml` project works with a bare `runtime: docker`.
`services` and `logs` are distinct axes.
`services` controls which compose services are started, and `logs` controls which running containers blink follows in the UI.
`stop_on_exit` defaults false so containers persist between runs and the next boot reuses warm databases; pre-existing containers blink did not start are never touched.

## TUI keybindings

The TUI's keys are data-driven.
A closed catalog of named actions is bound to keys by a default keymap, and `control.keys` in the config rebinds or unbinds any of them.

```yaml
control:
  keys:
    x: restart          # x now also restarts (r still works unless unbound)
    ctrl+r: restart-all
    enter: ""           # empty value unbinds a key
```

Key strings use bubbletea's form, for example `r`, `R`, `ctrl+c`, `shift+tab`, `enter`, `esc`, and a single space `" "` for the spacebar.
Overrides layer on top of the defaults, so keys you do not mention keep theirs.
Binding a key to an action name that is not in the catalog is a hard error at load time, so a typo fails the run rather than silently doing nothing.
Mapping a key to an empty string unbinds it and drops it from the help modal, which is how you turn a default binding off.

### Action catalog

Every action is bound out of the box by the default keymap.

| Action | Default key(s) | Description |
| --- | --- | --- |
| `restart` | `r` | restart the focused service |
| `restart-all` | `R` | restart all services |
| `insert-blank` | `enter` | insert a blank line into the focused output |
| `next-tab` | `right` | next tab |
| `prev-tab` | `left` | previous tab |
| `next-child` | `tab` | focus the next container (docker tab) |
| `prev-child` | `shift+tab` | focus the previous container (docker tab) |
| `hist-back` | `[` | back to the previously viewed tab |
| `hist-forward` | `]` | forward in tab history |
| `clear` | `k` | clear the focused tab buffer |
| `clear-all` | `K` | clear all buffers |
| `cursor-mode` | `e` | toggle cursor mode (line cursor and selection) |
| `cursor-up` | `up` | scroll up (cursor up in cursor mode) |
| `cursor-down` | `down` | scroll down (cursor down in cursor mode) |
| `extend-up` | `shift+up` | extend selection up |
| `extend-down` | `shift+down` | extend selection down |
| `toggle-select` | `space` | toggle the cursor line in the selection |
| `copy` | `c` | copy selection (or cursor line) to the clipboard |
| `clear-cursor` | `esc` | clear selection / exit cursor mode |
| `write-selection` | `w` | rewrite `<svc>.selected.log` with the selection |
| `append-selection` | `a` | append the selection to `<svc>.selected.log` |
| `toggle-logs` | `L` | toggle log-file writing |
| `command-center` | `/`, `?` | open the help / key-bindings modal |
| `toggle-zen` | `z` | toggle zen mode |
| `quit` | `q`, `ctrl+c` | quit |

Because `cursor-up`/`cursor-down` and the other catalog actions go through the keymap, the arrow keys are rebindable like any other action.

### Fixed navigation

A few log-navigation keys sit outside the rebindable keymap so they are always available.
`control.keys` cannot touch these.

| Key | Action |
| --- | --- |
| `1`-`9` | jump to tab |
| `pgup` / `pgdn` | page up / down |
| `home` / `end` | scroll to top / bottom |

Mouse and touchpad scrolling also work.
The bubbles viewport's own letter keys (`f`, `b`, `u`, `d`, `j`) are deliberately cleared so they do not silently scroll.
