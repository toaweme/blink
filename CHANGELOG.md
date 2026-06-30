# Changelog

All notable changes to this project are documented here, newest first.

Entries are generated from [Conventional Commits](https://www.conventionalcommits.org)
and grouped by change type. This project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2026-07-01

### Features

- Per-container ports in footer by Ignas Bernotas in [14b586e](https://github.com/toaweme/blink/commit/14b586edaf92d154f1bd435b3b07949b1c6da827).
- Show service address in tui footer by Ignas Bernotas in [6e401fc](https://github.com/toaweme/blink/commit/6e401fc3362249a711845bbf1b07e8f9155d8fdf).
- Multi-format blink config (yml/yaml/toml/json) with unified -c/--config flags by Ignas Bernotas in [4a3ad3b](https://github.com/toaweme/blink/commit/4a3ad3b55f2815c13945ef651b1524fa28928406).
- Add services from outside the project root by Ignas Bernotas in [9c1c3e4](https://github.com/toaweme/blink/commit/9c1c3e49e5b76b76e089a955ae2840067cfc6d23).
- Node runtime and setup lifecycle hook by Ignas Bernotas in [a33c02e](https://github.com/toaweme/blink/commit/a33c02ed0d19d501b9980dbe2a3b66ef5caf4c2f).
- Nuke command by Ignas Bernotas in [ec0b338](https://github.com/toaweme/blink/commit/ec0b338d17ecee055fd28980d015cfeea178fc9c).
- Runtime port discovery in the init/edit picker by Ignas Bernotas in [bf191df](https://github.com/toaweme/blink/commit/bf191df77a25fc6470583e27e0f6a252b8562730).
- Detect air services as go runtime by Ignas Bernotas in [4147499](https://github.com/toaweme/blink/commit/4147499452acf22fece85bc6ae20d0b42aa13b28).
- **Wip:** Init, edit commands by Ignas Bernotas in [5671408](https://github.com/toaweme/blink/commit/567140882d8304c70679ab48bc74d6322c2eb00d).
- Docker runtime with per-container log switching by Ignas Bernotas in [7d8adf3](https://github.com/toaweme/blink/commit/7d8adf35cf93e375107605699a4d1b09aaa3874b).

### Fixes

- Document intentional noctx/gosec exceptions for care lint by Ignas Bernotas in [bc92714](https://github.com/toaweme/blink/commit/bc927147823d93631e45acebb4608240fc99a532).
- Tui colors and widgets by Ignas Bernotas in [ffe6bf7](https://github.com/toaweme/blink/commit/ffe6bf78b3f849361cb74f137835266e31c8fc14).
- Canonical blink.yml config name and Test_ naming convention by Ignas Bernotas in [e674588](https://github.com/toaweme/blink/commit/e67458845f28e4486a22a20dd70ca5ed015078b4).
- Init + edit rendering by Ignas Bernotas in [a5ee13a](https://github.com/toaweme/blink/commit/a5ee13a75628c64cc0b47ff63af4f6a22603e69e).

### Refactors

- Cleanup unused code by Ignas Bernotas in [ebd8dfd](https://github.com/toaweme/blink/commit/ebd8dfd57268402c4b08ec9a2346cc26eb7a06bf).
- Tidy up blink config by Ignas Bernotas in [543dd70](https://github.com/toaweme/blink/commit/543dd70cf96fd85aa49530b8bce2238e4b4280b5).
- Centralize blink artifacts under .blink via Paths by Ignas Bernotas in [4c5689c](https://github.com/toaweme/blink/commit/4c5689c8725723dcb4033853bb109c74fc6a2bb3).

### CI & Build

- Bump care to v0.6.0 and fix card-svg dark/light wiring by Ignas Bernotas in [96ef96b](https://github.com/toaweme/blink/commit/96ef96bf668abf25d6598bef3e44f10523116903).

### Chores & Other

- Add README, LICENSE, CHANGELOG, and CI/release workflows by Ignas Bernotas in [25669df](https://github.com/toaweme/blink/commit/25669df2ce188c535faf453de8f31f275885d7fa).
- Tidy up by Ignas Bernotas in [3a18a20](https://github.com/toaweme/blink/commit/3a18a20060f129feaf48997df76eb4579880bc48).
- .golangci.yml + linter fixes + code comments by Ignas Bernotas in [7c457e2](https://github.com/toaweme/blink/commit/7c457e20e0903c26138a428f78345b402654f45a).
- Initial commit :) by Ignas Bernotas in [60839ba](https://github.com/toaweme/blink/commit/60839ba0804802bcf9d5d672d540e7f78ef0ceb3).

[0.1.0]: https://github.com/toaweme/blink/releases/tag/0.1.0
