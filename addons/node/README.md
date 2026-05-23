# node

The Node.js runtime. Auto-detects the package manager from lockfiles and
synthesizes a setup-install + run-script pair. Watches JS/TS source by default;
the install runs once on boot, not on every reload.

```
service dir         node.Runtime.Prepare()              supervisor lifecycle
+-----------+      +-----------------------------+    +-----------------------------------+
| pnpm-lock | ---> | pm: pnpm (from lockfile)    |    | setup: pnpm install   (boot only) |
| .yaml     |      | Plan.Defaults:              | -> | run:   pnpm run dev   (per reload)|
+-----------+      |   extensions: [js,ts,...]   |    +-----------------------------------+
                   |   setup: pnpm install       |
                   |   run:   pnpm run dev       |    SetupTriggers: [package.json]
                   +-----------------------------+    (edit re-runs setup; no loop, since
                                                       lockfiles are install outputs)
```

## Role

`runtime: node` is a defaults-only runtime: `Prepare` returns a `Plan.Defaults`
that the supervisor merges onto the user's service (explicit fields win). It
owns no process lifecycle of its own; the supervisor runs the synthesized
commands like any shell service.

## Package manager detection

Resolved by `config.NodePackageManager` (shared with the `detect` package so the
two never drift), in priority order:

1. `pnpm-lock.yaml` -> pnpm
2. `bun.lock` / `bun.lockb` -> bun
3. `yarn.lock` -> yarn
4. `package-lock.json` -> npm
5. (none) -> npm (fallback)

## Install lifecycle (the DX bit)

The install is emitted as a `Commands.Setup` step, not a per-run `Before` step.
The supervisor runs `Setup`:

- **once on boot**, and
- **again only when `package.json` changes** (declared via `Plan.SetupTriggers`),

never on an ordinary source-file reload. So editing a `.ts` file restarts the
dev server without reinstalling, while editing `package.json` (adding a
dependency) reinstalls before the restart.

Why `package.json` and not the lockfiles: a bare `<pm> install` *rewrites* the
lockfile, so triggering setup on lockfile changes would loop (install -> lockfile
write -> install). `package.json` is the human-authored dependency source and a
bare install never modifies it. Lockfiles are also excluded from the watch set
entirely, so install's own writes never cause a spurious reload. A lockfile-only
change (e.g. a teammate bumping a transitive dep) is picked up on the next blink
restart.

## Control / escape hatches

- `install: false` skips the managed install entirely.
- Author your own `commands.setup:` in blink.yaml; the runtime's install runs
  first, then yours (merge appends user setup after the runtime's).

## Options (`node:` block in blink.yaml)

| Field             | Type   | Default     | Description                                        |
|-------------------|--------|-------------|----------------------------------------------------|
| `script`          | string | `dev`       | npm script to run (`<pm> run <script>`)            |
| `package_manager` | string | auto-detect | Override: `npm`, `yarn`, `pnpm`, or `bun`          |
| `install`         | *bool  | true        | Emit `<pm> install` as a boot/manifest-only Setup step |
