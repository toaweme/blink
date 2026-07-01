# blink

[![Quality](https://github.com/toaweme/blink/actions/workflows/quality.yml/badge.svg)](https://github.com/toaweme/blink/actions/workflows/quality.yml)
<a href="https://code.toawe.me/toaweme/blink/health">
    <picture>
        <source media="(prefers-color-scheme: dark)" srcset="https://code.toawe.me/toaweme/blink/badge-dark.svg">
        <source media="(prefers-color-scheme: light)" srcset="https://code.toawe.me/toaweme/blink/badge.svg">
        <img alt="blink health" src="https://code.toawe.me/toaweme/blink/badge.svg">
    </picture>
</a>
[![GitHub Tag](https://img.shields.io/github/v/tag/toaweme/blink?label=Tag&color=green)](https://github.com/toaweme/blink/releases)
[![License](https://img.shields.io/badge/License-MIT-blue)](/LICENSE)

## Boot your dev stack

`blink` boots your every service, writes and prints logs, keeps everything alive, and restarts when files change or services restart. All behind beautiful TUI or a headless mode. 

Point it at a repo (or a few), run `blink`, and your shell scripts, Go binaries, Node apps, and Docker compose stacks come up together with multiplexed logs, per-service tabs, and port reclaiming. 

It detects what your project runs, so the common case needs zero config, and it stays fully offline.

## Install

```sh
# go
go install github.com/toaweme/blink/cmd/blink@latest

# homebrew
brew install toaweme/tap/blink

# binary (swap version/os/arch as needed)
wget -qO- https://github.com/toaweme/blink/releases/download/vX.Y.Z/blink_X.Y.Z_linux_x64.tar.gz | tar xz
```

Every release also lists the exact archive for each OS/arch on the
[releases page](https://github.com/toaweme/blink/releases).

## Usage

Run it in a project. With no `blink.yaml`, blink scans the directory, shows a checkbox of the services it detected, and runs your picks ephemerally (nothing is written):

```sh
blink            # same as `blink run`
```

Once you want a committed setup, scan and write a config:

```sh
blink init       # interactive picker, writes blink.yml
blink run        # supervise everything in blink.yml with live reload
```

`run` is the default command, so `blink` and `blink run` are the same thing.

## Commands

```sh
blink run        # supervise the services in blink.yaml with live reload (default)
blink init       # scan the project and write a new blink.yml
blink edit       # add/remove/modify services in an existing config
blink nuke       # remove blink's state (.blink dir + logs) so the next run is clean
blink help       # command help
```

### `blink run`

```sh
blink run -c blink.toml          # explicit config path (yml/yaml/toml/json)
blink run -s web,api             # only start a subset of services
blink run -u plain               # line-prefixed stdout instead of the TUI
blink run -u headless            # no UI at all (alias: -u none)
blink run -z                     # zen mode: native scrollback, no chrome
blink run -k off                 # don't kill processes bound to declared ports
blink run -l off                 # don't write per-service log files
```

| Flag | Short | Env | Meaning |
| --- | --- | --- | --- |
| `--config` | `-c` | `BLINK_CONFIG` | Config path; walks up from cwd when empty |
| `--ui` | `-u` | `BLINK_UI` | `blink`, `plain`, or `headless` (alias `none`) |
| `--services` | `-s` | `BLINK_SERVICES` | Comma-separated subset of services to start |
| `--zen` | `-z` | `BLINK_ZEN` | Start the TUI in zen mode |
| `--force-shutdown` | `-k` | `BLINK_FORCE_SHUTDOWN` | `on` kills anything on declared ports before start, `off` never does (default on) |
| `--logs` | `-l` | `BLINK_LOGS` | `on`/`off` per-service log files at `<LogDir>/<svc>.log` (default on) |

### `blink init` / `blink edit`

Both open the same compact picker over the detected (or existing) services: `space` selects, `→` edits a service, `a` adds one, `f` adds services from a sibling directory (e.g. `../ui`), `p` probes a service to discover the ports it binds, `enter` saves. `init` defaults to `blink.yml`; the extension you give `-c` picks the format (`.yml`/`.yaml`, `.toml`, `.json`), and `edit` round-trips whatever format it loaded.

```sh
blink init                       # writes blink.yml
blink init -c blink.toml         # writes TOML
blink init -f                    # overwrite an existing file
blink edit                       # edit the discovered config in place
```

### `blink nuke`

```sh
blink nuke                       # remove project state (.blink dir + logs), confirms first
blink nuke -y                    # skip the confirmation
blink nuke -g                    # also remove user-scoped ~/.blink (shared across projects)
```

By default it touches only project-scoped state under the current directory and prints what it kept.

## Configuration

`blink.yaml` lists the services to supervise. blink reads `blink.yml`, `blink.yaml`, `blink.toml`, or `blink.json`, discovered by walking up from the cwd. A `.env` in the project root is auto-loaded at startup (shell-set vars still win).

```yaml
services:
  - name: db
    runtime: docker          # drives `docker compose`, streams every container's logs

  - name: api
    runtime: go              # builds and runs a Go package, auto-watches go.work roots
    go:
      package: ./cmd/api
    ports: [8080]            # killed before start when force-shutdown is on
    reload:
      reload_on_service: [db]   # restart after db, and start after it

  - name: web
    runtime: node            # detects the package manager, runs `<pm> run dev`
    dir: ../ui               # services can live in sibling repos
    node:
      script: dev
```

Each service picks a `runtime`: `shell` (default, runs your `commands` as `sh -c`), `go`, `node`, or `docker`. A runtime contributes sensible defaults (build/run commands, watched extensions, install steps) that anything you set explicitly overrides. `reload_on_service` both orders startup and cascades restarts; `ports` drive port reclaiming; `commands.setup` runs once on boot and again only when a trigger file like `package.json` changes, keeping live reloads fast.

State (logs, build output) lives under a per-project `.blink/` dir; user-scoped config lives in `~/.blink`. Any path can be overridden via config or `$BLINK_*` env vars.

## TUI

The default `blink` UI gives one tab per service plus an `all` tab, color-coded by status (gray pending, red error, green running), with a footer showing watch stats, uptime, and each service's `http://127.0.0.1:<port>` address. Keys are data-driven and rebindable via `control.keys` in the config:

- Per-service tabs and a merged `all` view; `[`/`]` walk tab history.
- Scroll vs cursor mode (`e`); multi-line selection (`space`, `shift+↑/↓`) with copy (`c`), rewrite (`w`), and append (`a`) to `<LogDir>/<svc>.selected.log`, all to the clipboard too.
- Container switcher for docker services: `Tab`/`Shift+Tab` cycles focus to one container's clean logs.
- `L` toggles log-file writing live; a keyboard-help modal renders the live bindings.

## Features

- **Zero-config start** - no `blink.yaml` needed; blink scans the project, you tick the services it found, and they run ephemerally.
- **Multi-runtime supervision** - `shell`, `go`, `node`, and `docker` runtimes, each contributing build/run defaults you can override per service.
- **Live reload with a dependency DAG** - fsnotify-driven restarts with debounce, include roots and excludes; `reload_on_service` cascades restarts and orders startup.
- **Project detection** - detectors for go, air (mapped to the go runtime), docker, node, python, rust, and Procfile, with best-effort port discovery from `.env` files.
- **Runtime port probing** - the `init`/`edit` picker can spin a service up to observe the ports it actually binds and write them back (literal or `$ENV` reference).
- **Port reclaiming** - `force-shutdown` kills stale processes bound to a service's declared ports before it starts.
- **Docker compose lifecycle** - `up -d`, event-driven status, TCP readiness, multiplexed per-container logs, optional `stop_on_exit`.
- **Cross-repo services** - one config can supervise sibling directories (`../ui`), rebased relative to the project root.
- **Multi-format config** - `blink.yml`/`.yaml`/`.toml`/`.json`, read and written symmetrically; `init`/`edit` pick the format from the file extension.
- **Bubbletea TUI** - per-service tabs, an `all` view, container switcher, scroll/cursor modes, multi-line selection and copy, rebindable keymap, and a zen mode.
- **Pluggable UIs** - `blink` (TUI), `plain` (line-prefixed stdout), or `headless` (no UI), all writing per-service log files independent of the UI.
- **Scoped state and reset** - artifacts under a per-project `.blink/`, user-scoped `~/.blink`, all overridable via `$BLINK_*`, with `blink nuke` for a scoped wipe.
- **Fully offline** - no network calls at build or run time.

## Supporting blink

blink is free, open source (MIT), fully offline, and always will be. Nothing is
gated, time-limited, or hidden.

The only thing a license changes is cosmetic. It removes a small support badge
shown in the TUI. That is the whole deal. The tool behaves identically with
or without it, and there is no license check that phones home.

You never have to pay. Keep the badge, or build your own badge-free version from
source. A license is simply a way to say thanks if blink saves you time, and to
quiet that one badge. Pricing will be announced later.

## Hosted code and health reports

Reports for this repo are hosted by our <a href="https://code.toawe.me">code viewer</a>, which also serves the badges and cards above.

<p align="center">
  <a href="https://code.toawe.me/toaweme/blink/health"><picture><source media="(prefers-color-scheme: dark)" srcset="https://code.toawe.me/toaweme/blink/card-dark.svg"><source media="(prefers-color-scheme: light)" srcset="https://code.toawe.me/toaweme/blink/card.svg"><img alt="blink health" src="https://code.toawe.me/toaweme/blink/card.svg" width="48%"></picture></a>
  <a href="https://code.toawe.me/toaweme/blink/code"><picture><source media="(prefers-color-scheme: dark)" srcset="https://code.toawe.me/toaweme/blink/code-card-dark.svg"><source media="(prefers-color-scheme: light)" srcset="https://code.toawe.me/toaweme/blink/code-card.svg"><img alt="blink code" src="https://code.toawe.me/toaweme/blink/code-card.svg" width="48%"></picture></a>
</p>

---

Made with ❤️ in Lithuania 🇱🇹.
