# Changelog

All notable changes to this project are documented here, newest first.

Entries are generated from [Conventional Commits](https://www.conventionalcommits.org)
and grouped by change type. This project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.1] - 2026-07-01

### Chores & Other

- Bump cli to v0.3.3 by [@iberflow](https://github.com/iberflow) in [b463a79](https://github.com/toaweme/blink/commit/b463a790e2f144f7a2c9cbe919904322447f1e85).
- Bump toaweme deps to latest releases by [@iberflow](https://github.com/iberflow) in [80b1f91](https://github.com/toaweme/blink/commit/80b1f91063dfa623b0e43bea9ab6a5eda4fc79fe).

## [0.1.0] - 2026-07-01

### Features

- Docker runtime with per-container log switching by [@iberflow](https://github.com/iberflow) in [7d8adf3](https://github.com/toaweme/blink/commit/7d8adf35cf93e375107605699a4d1b09aaa3874b).
- **Wip:** Init, edit commands by [@iberflow](https://github.com/iberflow) in [5671408](https://github.com/toaweme/blink/commit/567140882d8304c70679ab48bc74d6322c2eb00d).
- Detect air services as go runtime by [@iberflow](https://github.com/iberflow) in [4147499](https://github.com/toaweme/blink/commit/4147499452acf22fece85bc6ae20d0b42aa13b28).
- Runtime port discovery in the init/edit picker by [@iberflow](https://github.com/iberflow) in [bf191df](https://github.com/toaweme/blink/commit/bf191df77a25fc6470583e27e0f6a252b8562730).
- Nuke command by [@iberflow](https://github.com/iberflow) in [ec0b338](https://github.com/toaweme/blink/commit/ec0b338d17ecee055fd28980d015cfeea178fc9c).
- Node runtime and setup lifecycle hook by [@iberflow](https://github.com/iberflow) in [a33c02e](https://github.com/toaweme/blink/commit/a33c02ed0d19d501b9980dbe2a3b66ef5caf4c2f).
- Add services from outside the project root by [@iberflow](https://github.com/iberflow) in [9c1c3e4](https://github.com/toaweme/blink/commit/9c1c3e49e5b76b76e089a955ae2840067cfc6d23).
- Multi-format blink config (yml/yaml/toml/json) with unified -c/--config flags by [@iberflow](https://github.com/iberflow) in [4a3ad3b](https://github.com/toaweme/blink/commit/4a3ad3b55f2815c13945ef651b1524fa28928406).
- Show service address in tui footer by [@iberflow](https://github.com/iberflow) in [6e401fc](https://github.com/toaweme/blink/commit/6e401fc3362249a711845bbf1b07e8f9155d8fdf).
- Per-container ports in footer by [@iberflow](https://github.com/iberflow) in [14b586e](https://github.com/toaweme/blink/commit/14b586edaf92d154f1bd435b3b07949b1c6da827).

### Fixes

- Init + edit rendering by [@iberflow](https://github.com/iberflow) in [a5ee13a](https://github.com/toaweme/blink/commit/a5ee13a75628c64cc0b47ff63af4f6a22603e69e).
- Canonical blink.yml config name and Test_ naming convention by [@iberflow](https://github.com/iberflow) in [e674588](https://github.com/toaweme/blink/commit/e67458845f28e4486a22a20dd70ca5ed015078b4).
- Tui colors and widgets by [@iberflow](https://github.com/iberflow) in [ffe6bf7](https://github.com/toaweme/blink/commit/ffe6bf78b3f849361cb74f137835266e31c8fc14).
- Document intentional noctx/gosec exceptions for care lint by [@iberflow](https://github.com/iberflow) in [bc92714](https://github.com/toaweme/blink/commit/bc927147823d93631e45acebb4608240fc99a532).
- Linter issues by [@iberflow](https://github.com/iberflow) in [92a315c](https://github.com/toaweme/blink/commit/92a315c2c024eda63afd6984e332e5de939264e4).
- Implement windows Runner start/kill, add cross-compile CI check by [@iberflow](https://github.com/iberflow) in [a993ceb](https://github.com/toaweme/blink/commit/a993cebd615ceae70892d010091fde585445cd88).

### Documentation

- Bump CHANGELOG by [@iberflow](https://github.com/iberflow) in [14c50f2](https://github.com/toaweme/blink/commit/14c50f2682208b16397e5ffbde6b690cf8fbafe5).

### Refactors

- Centralize blink artifacts under .blink via Paths by [@iberflow](https://github.com/iberflow) in [4c5689c](https://github.com/toaweme/blink/commit/4c5689c8725723dcb4033853bb109c74fc6a2bb3).
- Tidy up blink config by [@iberflow](https://github.com/iberflow) in [543dd70](https://github.com/toaweme/blink/commit/543dd70cf96fd85aa49530b8bce2238e4b4280b5).
- Cleanup unused code by [@iberflow](https://github.com/iberflow) in [ebd8dfd](https://github.com/toaweme/blink/commit/ebd8dfd57268402c4b08ec9a2346cc26eb7a06bf).

### CI & Build

- Bump care to v0.6.0 and fix card-svg dark/light wiring by [@iberflow](https://github.com/iberflow) in [96ef96b](https://github.com/toaweme/blink/commit/96ef96bf668abf25d6598bef3e44f10523116903).
- Bump care to v0.7.1 and pin to commit sha by [@iberflow](https://github.com/iberflow) in [f5a28b9](https://github.com/toaweme/blink/commit/f5a28b91f2f3d77c367f0fd79cfaddd08919a380).
- Pin the go-install care step by commit sha too by [@iberflow](https://github.com/iberflow) in [53c0abd](https://github.com/toaweme/blink/commit/53c0abd947f21e7b1da2122593943b54c10ef2eb).
- Bump care to v0.8.0 by [@iberflow](https://github.com/iberflow) in [066ef4c](https://github.com/toaweme/blink/commit/066ef4c069189c6c3225f4fd768425871e74754e).

### Chores & Other

- Initial commit :) by [@iberflow](https://github.com/iberflow) in [60839ba](https://github.com/toaweme/blink/commit/60839ba0804802bcf9d5d672d540e7f78ef0ceb3).
- .golangci.yml + linter fixes + code comments by [@iberflow](https://github.com/iberflow) in [7c457e2](https://github.com/toaweme/blink/commit/7c457e20e0903c26138a428f78345b402654f45a).
- Tidy up by [@iberflow](https://github.com/iberflow) in [3a18a20](https://github.com/toaweme/blink/commit/3a18a20060f129feaf48997df76eb4579880bc48).
- Add README, LICENSE, CHANGELOG, and CI/release workflows by [@iberflow](https://github.com/iberflow) in [25669df](https://github.com/toaweme/blink/commit/25669df2ce188c535faf453de8f31f275885d7fa).
- Bump deps by [@iberflow](https://github.com/iberflow) in [8678e08](https://github.com/toaweme/blink/commit/8678e08f4f4027ca1d983fa388704052e276a35e).
- Freeze go 1.26.4 by [@iberflow](https://github.com/iberflow) in [c846eee](https://github.com/toaweme/blink/commit/c846eeefe2e0ed9afc11748e172443acdfacc85e).

[0.1.1]: https://github.com/toaweme/blink/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/toaweme/blink/releases/tag/v0.1.0
