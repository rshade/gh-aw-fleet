# Changelog

## Unreleased

### Added

* **logging:** `--log-level` and `--log-format` persistent flags on the root command (values `trace|debug|info|warn|error` and `console|json`; defaults `info` and `console`); structured logging for errors, warnings, diagnostic hints, and subprocess summaries on stderr.

### Changed

* **logging:** `⚠ WARNING:` lines for missing Actions secrets (deploy) and workflow drift (sync) moved from stdout (tabwriter) to stderr (structured `warn`-level log events). Scripts that grepped stdout for `⚠ WARNING:` should switch to stderr. The `hint:` plaintext lines on stdout are unchanged and are additionally emitted as structured `warn` events on stderr.

## [0.1.0](https://github.com/rshade/gh-aw-fleet/compare/v1.0.0...v0.1.0) (2026-04-21)


### Added

* bootstrap gh-aw-fleet CLI, profiles, and operator skills ([e663c0c](https://github.com/rshade/gh-aw-fleet/commit/e663c0c98ab6f4ab024e752b2fb4ce3d4c7fdfa2))
* **cmd:** add `add <owner/repo>` subcommand for fleet onboarding ([#27](https://github.com/rshade/gh-aw-fleet/issues/27)) ([660f626](https://github.com/rshade/gh-aw-fleet/commit/660f62627f3c70ac984f506932b72cfe721299b8)), closes [#9](https://github.com/rshade/gh-aw-fleet/issues/9)


### Fixed

* **deploy:** check org-level secrets to avoid false-positive warnings ([93f43f7](https://github.com/rshade/gh-aw-fleet/commit/93f43f7d5df318507e1e418d4ee2b8349407e780))
* **release:** align release-please with single-package layout ([cc9027c](https://github.com/rshade/gh-aw-fleet/commit/cc9027ce2c3ee62ac693f64139885a368ac58082))
* **release:** restore release-as: 0.1.0 wiped by cc9027c ([8eac94b](https://github.com/rshade/gh-aw-fleet/commit/8eac94bc497b699cbfffe36cf44176890116d17c))


### Documentation

* **project:** add CONTEXT.md constitution and ROADMAP.md ([2d8bc88](https://github.com/rshade/gh-aw-fleet/commit/2d8bc885d7f5437c268204964a5c4076c0406684))

## 1.0.0 (2026-04-20)


### Added

* bootstrap gh-aw-fleet CLI, profiles, and operator skills ([e663c0c](https://github.com/rshade/gh-aw-fleet/commit/e663c0c98ab6f4ab024e752b2fb4ce3d4c7fdfa2))
* **cmd:** add `add <owner/repo>` subcommand for fleet onboarding ([#27](https://github.com/rshade/gh-aw-fleet/issues/27)) ([660f626](https://github.com/rshade/gh-aw-fleet/commit/660f62627f3c70ac984f506932b72cfe721299b8)), closes [#9](https://github.com/rshade/gh-aw-fleet/issues/9)


### Fixed

* **deploy:** check org-level secrets to avoid false-positive warnings ([93f43f7](https://github.com/rshade/gh-aw-fleet/commit/93f43f7d5df318507e1e418d4ee2b8349407e780))
* **release:** align release-please with single-package layout ([cc9027c](https://github.com/rshade/gh-aw-fleet/commit/cc9027ce2c3ee62ac693f64139885a368ac58082))


### Documentation

* **project:** add CONTEXT.md constitution and ROADMAP.md ([2d8bc88](https://github.com/rshade/gh-aw-fleet/commit/2d8bc885d7f5437c268204964a5c4076c0406684))
