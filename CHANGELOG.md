# Changelog

## Unreleased

### Added

* **logging:** `--log-level` and `--log-format` persistent flags on the root command (values `trace|debug|info|warn|error` and `console|json`; defaults `info` and `console`); structured logging for errors, warnings, diagnostic hints, and subprocess summaries on stderr.

### Changed

* **logging:** `⚠ WARNING:` lines for missing Actions secrets (deploy) and workflow drift (sync) moved from stdout (tabwriter) to stderr (structured `warn`-level log events). Scripts that grepped stdout for `⚠ WARNING:` should switch to stderr. The `hint:` plaintext lines on stdout are unchanged and are additionally emitted as structured `warn` events on stderr.

## [0.2.6](https://github.com/rshade/gh-aw-fleet/compare/v0.2.5...v0.2.6) (2026-07-01)


### Fixed

* **deps:** update module github.com/rshade/ax-go to v0.3.0 ([#178](https://github.com/rshade/gh-aw-fleet/issues/178)) ([2d5dc3e](https://github.com/rshade/gh-aw-fleet/commit/2d5dc3e63d7254e14f2e0eed6be85b4e2be47754))


### Documentation

* add tutorial/how-to/reference pages and de-duplicate README ([#184](https://github.com/rshade/gh-aw-fleet/issues/184)) ([b7c04fc](https://github.com/rshade/gh-aw-fleet/commit/b7c04fc880a288e5f3b200b0eb661f7ca3589913))

## [0.2.5](https://github.com/rshade/gh-aw-fleet/compare/v0.2.4...v0.2.5) (2026-06-30)


### Added

* **consumption:** read-only over-budget highlighting (--budget) ([#160](https://github.com/rshade/gh-aw-fleet/issues/160)) ([0bf134c](https://github.com/rshade/gh-aw-fleet/commit/0bf134c0b9e92bb6671e39ff8e67a8d293188a7b)), closes [#129](https://github.com/rshade/gh-aw-fleet/issues/129)
* **overview:** add read-only drift, run-health, and cost dashboard ([#172](https://github.com/rshade/gh-aw-fleet/issues/172)) ([21077df](https://github.com/rshade/gh-aw-fleet/commit/21077df3172131413581dd9d5e2bf8e94f52afc0)), closes [#153](https://github.com/rshade/gh-aw-fleet/issues/153)
* **security:** add interactive findings confirmation and --yes flag ([#177](https://github.com/rshade/gh-aw-fleet/issues/177)) ([aac0fdb](https://github.com/rshade/gh-aw-fleet/commit/aac0fdba230ed8b00b7b0825b0a069945d31eaaf)), closes [#40](https://github.com/rshade/gh-aw-fleet/issues/40)
* **security:** add opt-in --strict gate for HIGH Layer 1 findings ([#161](https://github.com/rshade/gh-aw-fleet/issues/161)) ([c0d8776](https://github.com/rshade/gh-aw-fleet/commit/c0d8776b3b877fa0283fbcc4ef5551b1483f335d)), closes [#38](https://github.com/rshade/gh-aw-fleet/issues/38)


### Fixed

* **deps:** update dependency @astrojs/starlight to ^0.41.0 ([#168](https://github.com/rshade/gh-aw-fleet/issues/168)) ([145dd35](https://github.com/rshade/gh-aw-fleet/commit/145dd35ac039bd3d69bf97e866d0b2225684e2c3))
* **deps:** update dependency starlight-links-validator to ^0.25.0 ([#169](https://github.com/rshade/gh-aw-fleet/issues/169)) ([6a0325c](https://github.com/rshade/gh-aw-fleet/commit/6a0325cb3766b29188cb8cef1deaee3ae5f990a5))

## [0.2.4](https://github.com/rshade/gh-aw-fleet/compare/v0.2.3...v0.2.4) (2026-06-22)


### Added

* **ax:** adopt ax-go config primitives and hidden schema command ([#159](https://github.com/rshade/gh-aw-fleet/issues/159)) ([ad55478](https://github.com/rshade/gh-aw-fleet/commit/ad55478960f1fc2bd413a1d143159e0ba0be0d6b)), closes [#156](https://github.com/rshade/gh-aw-fleet/issues/156)
* **pkg/fleet:** export fleet.json config contract as public package ([#152](https://github.com/rshade/gh-aw-fleet/issues/152)) ([980ceb1](https://github.com/rshade/gh-aw-fleet/commit/980ceb175e745a2564dc563e22fd54e937399f50)), closes [#148](https://github.com/rshade/gh-aw-fleet/issues/148)


### Documentation

* **site:** scaffold Astro Starlight docs site consuming rshade-theme ([#149](https://github.com/rshade/gh-aw-fleet/issues/149)) ([7fd2aec](https://github.com/rshade/gh-aw-fleet/commit/7fd2aec2249f06dba148cb8cee1e4d8699a68495)), closes [#138](https://github.com/rshade/gh-aw-fleet/issues/138)

## [0.2.3](https://github.com/rshade/gh-aw-fleet/compare/v0.2.2...v0.2.3) (2026-06-18)


### Added

* **security:** add advisory Dependabot config conflict scanner ([#135](https://github.com/rshade/gh-aw-fleet/issues/135)) ([fc3f48b](https://github.com/rshade/gh-aw-fleet/commit/fc3f48bff6ede41922ed8dd5133ab786ad0aa5f5)), closes [#101](https://github.com/rshade/gh-aw-fleet/issues/101)
* **security:** add advisory Renovate config conflict scanner ([#133](https://github.com/rshade/gh-aw-fleet/issues/133)) ([bfa1a30](https://github.com/rshade/gh-aw-fleet/commit/bfa1a306ed875aa8e60fba36ffa0fbc5f5c71af1)), closes [#100](https://github.com/rshade/gh-aw-fleet/issues/100)


### Fixed

* **deploy:** add init drift guard and upgrade init/manifest parity ([#130](https://github.com/rshade/gh-aw-fleet/issues/130)) ([8301ab0](https://github.com/rshade/gh-aw-fleet/commit/8301ab0471754272e927cee3e9e91f80cf94ea69)), closes [#98](https://github.com/rshade/gh-aw-fleet/issues/98)

## [0.2.2](https://github.com/rshade/gh-aw-fleet/compare/v0.2.1...v0.2.2) (2026-06-13)


### Added

* **consumption:** scope rollup to named repos via positional args ([#126](https://github.com/rshade/gh-aw-fleet/issues/126)) ([c1e3f38](https://github.com/rshade/gh-aw-fleet/commit/c1e3f3881e692c31dbd683cf3976853063ad0b6b))
* **consumption:** source AI credits from gh aw logs --json ([ce1e031](https://github.com/rshade/gh-aw-fleet/commit/ce1e0313e08f838b9321d82de74e9368884bc7ee)), closes [#103](https://github.com/rshade/gh-aw-fleet/issues/103) [#108](https://github.com/rshade/gh-aw-fleet/issues/108)
* **consumption:** source AI credits from gh aw logs --json ([#116](https://github.com/rshade/gh-aw-fleet/issues/116)) ([ce1e031](https://github.com/rshade/gh-aw-fleet/commit/ce1e0313e08f838b9321d82de74e9368884bc7ee))

## [0.2.1](https://github.com/rshade/gh-aw-fleet/compare/v0.2.0...v0.2.1) (2026-06-10)


### Added

* **consumption:** aggregate api-consumption-report output across the… ([#83](https://github.com/rshade/gh-aw-fleet/issues/83)) ([06ca083](https://github.com/rshade/gh-aw-fleet/commit/06ca083dafaaee368362a860201897facfd6cc38)), closes [#57](https://github.com/rshade/gh-aw-fleet/issues/57)
* **deploy/upgrade:** compile with --strict on public repos by default ([#88](https://github.com/rshade/gh-aw-fleet/issues/88)) ([ab598c0](https://github.com/rshade/gh-aw-fleet/commit/ab598c0516766d604691bacd473e466826ea4c17)), closes [#49](https://github.com/rshade/gh-aw-fleet/issues/49)
* **install:** add install.sh and install.ps1 one-liner installers ([#92](https://github.com/rshade/gh-aw-fleet/issues/92)) ([e008ec6](https://github.com/rshade/gh-aw-fleet/commit/e008ec6b263fa5781defd33a90d7c70cc781c142)), closes [#43](https://github.com/rshade/gh-aw-fleet/issues/43)

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
