# Changelog

## Unreleased

### Added

* **logging:** `--log-level` and `--log-format` persistent flags on the root command (values `trace|debug|info|warn|error` and `console|json`; defaults `info` and `console`); structured logging for errors, warnings, diagnostic hints, and subprocess summaries on stderr.

### Changed

* **logging:** `⚠ WARNING:` lines for missing Actions secrets (deploy) and workflow drift (sync) moved from stdout (tabwriter) to stderr (structured `warn`-level log events). Scripts that grepped stdout for `⚠ WARNING:` should switch to stderr. The `hint:` plaintext lines on stdout are unchanged and are additionally emitted as structured `warn` events on stderr.

## [0.2.0](https://github.com/rshade/gh-aw-fleet/compare/v0.1.4...v0.2.0) (2026-05-14)


### Added

* **config:** hujson syntax + billing-metadata fields ([#78](https://github.com/rshade/gh-aw-fleet/issues/78)) ([5b3f6fb](https://github.com/rshade/gh-aw-fleet/commit/5b3f6fbc2d87deaa04df1e9c19273865f577cec6)), closes [#54](https://github.com/rshade/gh-aw-fleet/issues/54) [#55](https://github.com/rshade/gh-aw-fleet/issues/55) [#73](https://github.com/rshade/gh-aw-fleet/issues/73)


### Fixed

* **sync:** lock in resume-guard bypass on internally-prepared clones ([#81](https://github.com/rshade/gh-aw-fleet/issues/81)) ([3eeb8b5](https://github.com/rshade/gh-aw-fleet/commit/3eeb8b56295958673c0095bd22c77c3e91ce7a65)), closes [#48](https://github.com/rshade/gh-aw-fleet/issues/48)


### Chores

* release 0.2.0 ([c416f89](https://github.com/rshade/gh-aw-fleet/commit/c416f8911c90383f27ab493145e047941ae3fc8e))

## [0.1.4](https://github.com/rshade/gh-aw-fleet/compare/v0.1.3...v0.1.4) (2026-05-10)


### Added

* **billing:** observability-plus profile + HTTP 402 diagnostic ([#72](https://github.com/rshade/gh-aw-fleet/issues/72)) ([560621e](https://github.com/rshade/gh-aw-fleet/commit/560621ea90387cdb64dbdb2b6b53acab58906283)), closes [#52](https://github.com/rshade/gh-aw-fleet/issues/52) [#56](https://github.com/rshade/gh-aw-fleet/issues/56)
* **security:** add Layer 1 scanner for secrets and structural rules ([#69](https://github.com/rshade/gh-aw-fleet/issues/69)) ([25937e8](https://github.com/rshade/gh-aw-fleet/commit/25937e829af9d02ee5fcae393caf04c5f4fe8bbd)), closes [#37](https://github.com/rshade/gh-aw-fleet/issues/37)

## [0.1.3](https://github.com/rshade/gh-aw-fleet/compare/v0.1.2...v0.1.3) (2026-05-01)


### Added

* **cmd:** implement status subcommand for fleet drift detection ([#64](https://github.com/rshade/gh-aw-fleet/issues/64)) ([ab116dc](https://github.com/rshade/gh-aw-fleet/commit/ab116dcf4b7de1dc2e98986440158bb43a26078e)), closes [#10](https://github.com/rshade/gh-aw-fleet/issues/10)
* **deploy:** preflight Actions-enabled and workflow-token-write ([#66](https://github.com/rshade/gh-aw-fleet/issues/66)) ([0694ae9](https://github.com/rshade/gh-aw-fleet/commit/0694ae93355a1652c1c7b82f9a9962ec393543f5)), closes [#11](https://github.com/rshade/gh-aw-fleet/issues/11)
* **deploy:** surface missing-secret warning in PR body ([#51](https://github.com/rshade/gh-aw-fleet/issues/51)) ([20b716b](https://github.com/rshade/gh-aw-fleet/commit/20b716bb52f90670e81b6343d7db874371837a7a)), closes [#7](https://github.com/rshade/gh-aw-fleet/issues/7)

## [0.1.2](https://github.com/rshade/gh-aw-fleet/compare/v0.1.1...v0.1.2) (2026-04-28)


### Added

* **deploy:** resume --work-dir at commit or push gate ([#50](https://github.com/rshade/gh-aw-fleet/issues/50)) ([f904148](https://github.com/rshade/gh-aw-fleet/commit/f90414875c742f21b806279605eafa91cb940a5b)), closes [#8](https://github.com/rshade/gh-aw-fleet/issues/8) [#32](https://github.com/rshade/gh-aw-fleet/issues/32)


### Fixed

* **cli:** name both config files in untracked-repo error ([#45](https://github.com/rshade/gh-aw-fleet/issues/45)) ([891377d](https://github.com/rshade/gh-aw-fleet/commit/891377de0e917753556b01d2bdef262078f453b2)), closes [#31](https://github.com/rshade/gh-aw-fleet/issues/31) [#30](https://github.com/rshade/gh-aw-fleet/issues/30)

## [0.1.1](https://github.com/rshade/gh-aw-fleet/compare/v0.1.0...v0.1.1) (2026-04-23)


### Added

* **cli:** add --output json mode for list/deploy/sync/upgrade ([#44](https://github.com/rshade/gh-aw-fleet/issues/44)) ([e4fbe91](https://github.com/rshade/gh-aw-fleet/commit/e4fbe91b189b331465625654fbf12b047ae2eeb6)), closes [#25](https://github.com/rshade/gh-aw-fleet/issues/25)
* **logging:** introduce zerolog for errors, warnings, and diagnostics ([#34](https://github.com/rshade/gh-aw-fleet/issues/34)) ([b6ab3b8](https://github.com/rshade/gh-aw-fleet/commit/b6ab3b870c6b3b7f47732e709b94b2f62a46637f)), closes [#24](https://github.com/rshade/gh-aw-fleet/issues/24)

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
