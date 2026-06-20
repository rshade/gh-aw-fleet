# Implementation Plan: Astro Starlight Documentation Site (Reference Implementation)

**Branch**: `014-starlight-docs-site` | **Date**: 2026-06-20 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/014-starlight-docs-site/spec.md`

## Summary

Scaffold an Astro Starlight documentation site under `docs/`, served from GitHub
Pages at the project subpath `https://rshade.github.io/gh-aw-fleet/`. The site
consumes brand design tokens from `rshade-theme` (a git submodule at `docs/theme`)
through a small CSS bridge, and is published by a path-scoped GitHub Actions
workflow (`docs.yml`, scoped to `docs/**`, `.gitmodules`, and itself) that is
independent of the Go release pipeline. The
implementation copies the wiring proven in the sibling `../ax-go/` repo
**verbatim except for one deliberate correction**: ax-go's checked-in theme
bridge maps the brand accent only under `:root`, which lets light mode revert to
Starlight's default accent — the exact defect this reference implementation
exists to fix (FR-005). We use the corrected both-scheme bridge the spec
designates as authoritative. We additionally ship a base-path-aware `robots.txt`
and an `llms.txt` (which ax-go does not yet have) for SEO/agent discoverability.

The five seed content pages — install/quickstart, `fleet.json` configuration
reference, the `deploy`/`sync`/`upgrade` reconcile workflow, FinOps/consumption
(migrated from the existing `docs/finops.md`), and the roadmap — are authored from
committed project material (README, `fleet.example.json`, `AGENTS.md`,
`docs/finops.md`, `ROADMAP.md`).

## Technical Context

**Language/Version**: Node.js 24 (CI pin; no `engines` field in the manifest) driving Astro 6 / TypeScript (`astro/tsconfigs/strict`). The Go module is untouched.
**Primary Dependencies** (docs project, isolated under `docs/package.json` — version ranges match `../ax-go/`; the committed `docs/package-lock.json` pins resolved versions):
- `astro` `^6.4.5` (lockfile resolves `6.4.8`)
- `@astrojs/starlight` `^0.40.0`
- `@astrojs/sitemap` `^3.7.3`
- `starlight-links-validator` `^0.24.1` (build-time broken-internal-link gate → SC-006)
- `sharp` `^0.35.0`

**Storage**: N/A — static site generator. Content is markdown/MDX under `docs/src/content/docs/`; build output is `docs/dist/` (gitignored). No database, no runtime backend.
**Testing**: `starlight-links-validator` fails the `astro build` on any broken internal link; a `prebuild` npm guard fails fast if the `docs/theme` submodule's `tokens.css` is missing. The Pages deploy job is the integration surface (publishes `docs/dist`). No Go tests are added or changed.
**Target Platform**: GitHub Pages (Pages-from-Actions), project subpath — `site: 'https://rshade.github.io'`, `base: '/gh-aw-fleet'`.
**Project Type**: Static documentation sub-project added to an existing Go CLI repo — a self-contained Node/Astro project rooted at `docs/`, independent of `go.mod` and goreleaser.
**Performance Goals**: N/A for the constitution's 5-minute CLI ceiling (this is a CI build, not a `gh-aw-fleet` command). Build should complete well within a normal Pages-deploy budget; `actions/setup-node` npm caching keyed on `docs/package-lock.json` keeps installs fast.
**Constraints**:
- All emitted URLs MUST carry the `/gh-aw-fleet` base (FR-003 / SC-006) — no domain-root assumptions.
- The brand accent MUST be set for **both** `:root` and `:root[data-theme="light"]` (FR-005); the authoritative bridge snippet from issue #138 overrides ax-go's single-scheme checked-in file.
- The shared theme MUST be present at build time via `submodules: recursive` checkout (FR-009); a missing submodule must fail loudly, not silently drop branding.
- The docs pipeline MUST NOT cross-trigger the Go release pipeline and vice versa (FR-007).
**Scale/Scope**: ~6 routes at launch (splash landing + 5 reference pages); single-version, default Pagefind search, no localization (per spec Out of Scope).

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Evaluated against `.specify/memory/constitution.md` v1.1.0. **Result: PASS — no violations, no Complexity Tracking required.**

| Principle / Section | Verdict | Notes |
|---|---|---|
| I. Thin-Orchestrator Code Quality | ✅ PASS | No Go code added or changed; `make ci` remains the local completion gate and should stay green because nothing under the Go module is touched. The docs site is not orchestration logic. |
| II. Testing (Build-Green + Dry-Run) | ✅ PASS | No mutating `gh-aw-fleet` command is added, so the dry-run requirement is not implicated. The docs analog of "build-green" is enforced: `astro build` + `starlight-links-validator` + the `prebuild` submodule guard must pass in CI. |
| III. UX Consistency (Three-Turn Mutation) | ✅ PASS | No new CLI command mutating external repos. Publishing is a CI pipeline on merge to `main`, not an interactive `--apply` flow. |
| IV. Performance (Parallelism, Cache, 5-min) | ✅ PASS | The 5-minute ceiling governs `gh-aw-fleet` invocations, not CI. npm cache is keyed on `docs/package-lock.json`. |
| Declarative Reconcile Invariants | ✅ PASS | `fleet.json`/`fleet.local.json`, gpg-signing, and the `git add/commit/push`-from-Bash ban are untouched. The one-time `git submodule add docs/theme` is operator setup at implementation time, not tool behavior. |
| **Third-Party Dependencies** (Go `go.mod`) | ✅ PASS — **not implicated** | The new dependencies live in `docs/package.json`, a separate Node project. The constitution governs the Go module's `require()` blocks only; `go.mod` gains nothing. Spec Assumptions ("Independent dependency surface") state this explicitly. |
| Development Workflow | ✅ PASS | `CLAUDE.md` is updated (agent-context marker) per the workflow rule. `release-please` continues to own `CHANGELOG.md`; the feature lands via a conventional-commit PR. |

**Out-of-scope guardrails honored**: no changes to the CLI, the Go build, goreleaser, or release-please config (spec Out of Scope). The docs pipeline is added alongside, never edited into, the Go pipelines.

## Project Structure

### Documentation (this feature)

```text
specs/014-starlight-docs-site/
├── plan.md              # This file (/speckit-plan command output)
├── research.md          # Phase 0 output — ax-go wiring extraction + decisions
├── data-model.md        # Phase 1 output — content/theme/pipeline entities
├── quickstart.md        # Phase 1 output — local dev + FR-010 "how this is wired"
├── contracts/           # Phase 1 output — published-routes, theme-bridge, pipeline contracts
│   ├── published-routes.md
│   ├── theme-bridge.md
│   └── docs-pipeline.md
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

The feature is a self-contained Astro project rooted at `docs/`, plus one new
workflow and three repo-root hygiene edits. Nothing under the Go module changes.

```text
docs/                                 # Astro project root (existing dir; gains the project)
├── astro.config.mjs                  # site/base + starlight + sitemap + links-validator
├── package.json                      # docs deps (astro, starlight, sitemap, links-validator, sharp) + prebuild guard
├── package-lock.json                 # committed lockfile (npm ci in CI)
├── tsconfig.json                     # extends astro/tsconfigs/strict
├── .gitignore                        # dist/, .astro/, node_modules/
├── theme/                            # git submodule → github.com/rshade/rshade-theme (provides tokens.css)
├── public/
│   ├── favicon.svg                   # brand favicon (light/dark via prefers-color-scheme)
│   ├── robots.txt                    # NEW vs ax-go — base-aware Sitemap: line
│   └── llms.txt                      # NEW vs ax-go — agent-discoverability index
├── src/
│   ├── content.config.ts             # docsLoader() + docsSchema()
│   ├── styles/
│   │   └── theme-bridge.css          # CORRECTED both-scheme accent + font bridge (FR-005)
│   └── content/
│       └── docs/
│           ├── index.mdx             # splash landing (hero + CardGrid)
│           ├── install.md            # FR-002(a) install/quickstart
│           ├── configuration.md      # FR-002(b) fleet.json reference
│           ├── reconcile.md          # FR-002(c) deploy/sync/upgrade
│           ├── consumption.md        # FR-002(d) migrated from docs/finops.md
│           └── roadmap.md            # FR-002(e) from ROADMAP.md
└── finops.md                         # EXISTING raw doc — migrated into src/content/docs/, then removed (FR-011/SC-007)

.github/workflows/
└── docs.yml                          # NEW path-scoped Pages deploy (docs/** + .gitmodules); submodules: recursive

.gitmodules                           # NEW — declares docs/theme → rshade-theme
.gitignore                            # EDIT — add node_modules/, docs/.astro/ (dist/ already present)
.markdownlintignore                   # NEW — exclude docs/theme, docs/dist, docs/.astro, docs/node_modules
```

**Structure Decision**: Single self-contained Node/Astro sub-project at `docs/`,
mirroring `../ax-go/docs/`. This keeps the documentation dependency surface fully
isolated from the Go module (FR-008) and gives sibling projects a directory they
can copy wholesale (FR-010). The Go layout (`cmd/`, `internal/`, root `*.go`) is
unchanged; this plan adds no Go files.

## Complexity Tracking

> No constitutional violations — this section is intentionally empty.

The Constitution Check passes on every principle with no justified exceptions, so
there is no added abstraction, no new `.claude/settings.json` deny/allow entry,
and no new Go direct dependency to track here.
