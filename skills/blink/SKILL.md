---
name: blink
description: Configure and run blink, the dev-stack supervisor that boots shell, Go, Node and Docker compose services together with live reload, port reclaiming and per-service log files. Use this whenever a project has or needs a `blink.yml` / `blink.yaml` / `blink.toml` / `blink.json`, when someone asks to "set up blink", "add a service to blink", "make blink reload on X", "why doesn't blink restart", "run the stack headless", or wants an agent to boot a local dev stack and read its logs. Also use it when a repo's `.blink/` dir or `blink run` output shows up in a task.
---

# blink for agents

blink supervises a project's services from one config file. An agent's job is usually one of three things, which are writing or editing that config, running the stack without a TTY, and reading service logs to debug.

The full field reference lives in `CONFIGURATION.md` in the blink repo (`github.com/toaweme/blink`). This skill is the working subset plus the traps.

## Rules for agents

- **Write the config by hand.** `blink init` and `blink edit` are interactive pickers and need a TTY. Zero-config `blink` also opens a picker. Never run those from an agent. Detect the services yourself (look for `go.mod` + `cmd/*`, `package.json` scripts, `compose.yaml`, `Procfile`) and write `blink.yml`.
- **Never run the TUI.** Use `blink run -u headless` or `blink run -u plain`. With no `-u` and no `ui:` in the config, blink picks `plain` when stdout is not a TTY, but be explicit.
- **Run it in the background.** `blink run` supervises forever. Start it as a background process and read logs from files, never block on it.
- **Read logs from files, not stdout.** Each service writes `.blink/logs/<svc>.log` (under `<dir_root>/.blink/logs` by default). `grep` or `tail` the one you need.
- **Mind port reclaiming.** `force_shutdown` defaults on, so blink SIGTERM/SIGKILLs whatever holds a service's declared `ports` before starting it. If the user already has something running on those ports that must survive, run with `-k off`.
- **`.env` is loaded automatically** from the project root at startup (shell-set vars win). Do not read `.env` contents to configure blink. Reference ports by env name (`ports: [PORT]`) instead of copying values.

## Running

```sh
blink run -u headless              # everything in the discovered config, no UI
blink run -u plain -s api,web      # subset, line-prefixed stdout
blink run -c path/to/blink.yml     # explicit config, skips walk-up
blink run -k off                   # do not kill processes on declared ports
blink nuke -y                      # wipe .blink/ (logs, build output) for a clean run
```

Config discovery walks up from cwd and takes the first of `blink.yml`, `blink.yaml`, `blink.toml`, `blink.json`. The extension picks the format. A named-but-missing `-c` path or a parse error fails hard, which is the fastest way to validate a config you just wrote.

## Minimal config

```yaml
services:
  - name: db
    runtime: docker            # docker compose up, streams container logs

  - name: api
    runtime: go
    go:
      package: ./cmd/api       # required for go
    ports: [PORT]              # env name or literal, reclaimed before start
    reload:
      reload: true
      reload_on_service: [db]  # start after db, restart when db restarts

  - name: web
    runtime: node
    dir: ../ui                 # relative to dir_root (the config file's dir)
    ports: [5173]
```

Keep it compact. Omit anything that matches the default. Every runtime default is overridable by setting the field explicitly.

## Picking a runtime

| Project shape | Runtime | What it gives you |
| --- | --- | --- |
| Go `main` package | `go` | `go build -o .blink/build/<name> <package>` then runs the binary. Watches `go`, `mod`, `sum`. Adds every `go.work` module as a watch root. |
| `package.json` | `node` | `<pm> run <script>` (`dev`, else `start`). PM from lockfile. `<pm> install` as a setup step. HMR dev servers (vite, next, nodemon, `tsx watch`) only restart on `package.json` changes. |
| compose file | `docker` | `docker compose up --wait`, event-driven status, multiplexed logs. Containers stay up after exit unless `stop_on_exit: true`. No file reload. |
| anything else | `shell` (default) | Runs `commands` as `sh -c`. No defaults at all. |

## Shell services

A shell service gets nothing for free. For a long-running process that should reload on edits, set all three of `commands.run`, `reload.reload` and `fs`.

```yaml
  - name: worker
    commands:
      setup:
        - command: pip install -r requirements.txt   # once, not on reload
      build:
        command: make worker                         # every boot and restart
      run:
        command: ./bin/worker
        service: true
    reload:
      reload: true
    fs:
      extensions: [py]
      include: [../shared]       # extra watch roots, relative to dir_root
      exclude: ["**/fixtures/**"]
```

## Traps

- **No reload without `reload: true`.** The runtime does not imply it. A service without it never restarts on edits, and blink only logs an info hint.
- **`build` runs on every restart.** Put dependency installs in `setup`, which reruns only when a runtime trigger file (`go.mod`, `package.json`) changes. For shell services `setup` runs once per boot.
- **blink never respawns a crashed process.** Restarts come only from file changes, a `reload_on_service` dependency restarting, or `r` in the TUI. To recover, fix the code (the watcher restarts it) or restart `blink run`.
- **Built-in excludes** are `.git`, `node_modules`, `dist`, `build`, `.next`, `.idea`, `.vscode`, `.blink` at any depth. Source in a dir named `build` or `dist` is never watched.
- **File-only `include`** (every entry a file) drops the service dir as a watch root. Mix in a directory or list the service dir if you still want source edits to trigger.
- **`reload_on_service`** names must exist and must not cycle, both checked at load. A dependency that crashes on boot marks the dependent `crashed` instead of hanging.
- **Env-referenced ports** resolve from the service `env` first, then the process env (including `.env`). An unresolved name is dropped with a warning, so the port is not reclaimed.
- **`force_shutdown` and `logs.write` are nullable.** Leaving them out means default (on), not false.
- **`command_cleanup`** only works on `commands.run`.
- **`disabled: true`** keeps a service in the file but out of the run. Prefer it over deleting a service the user may want back.

## State and paths

| Path | Default | Env override |
| --- | --- | --- |
| project state | `<dir_root>/.blink` | `BLINK_CONTROL_DIR` |
| logs | `.blink/logs/<svc>.log` | `BLINK_LOG_DIR` |
| go build output | `.blink/build/<svc>` | `BLINK_BUILD_DIR` |
| user state | `~/.blink` | `BLINK_CONFIG_HOME` |
| config path | walk up from cwd | `BLINK_CONFIG` |

`.blink/` belongs in `.gitignore`. Check before committing a new `blink.yml`.

## Other fields worth knowing

- `env` (map, per service) is merged into every command's environment and beats runtime-contributed keys.
- `hostname` (per service, default `localhost`) is the host the TUI opens in a browser.
- `go.args`, `go.out`, `go.workspace: false` tune the go runtime.
- `node.script`, `node.package_manager` (`npm`, `pnpm`, `yarn`, `bun`), `node.install: false` tune the node runtime.
- `docker.file`, `docker.project`, `docker.services` (which to start), `docker.logs` (which to follow), `docker.wait`, `docker.stop_on_exit`, `docker.log_tail` tune the docker runtime.
- `reload.reload_on_delete` (globs) restarts when a matching file is removed, even with `reload: false`.
- `control.keys` rebinds TUI keys. Irrelevant headless, but an unknown action name fails the load.
