# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Common commands

```bash
go build ./...        # Compile. Run before claiming a change builds.
go vet ./...          # Must pass before any edit is considered done.
go run . <cmd>        # Run the CLI directly (e.g. `go run . list`).
go run . list         # Fastest sanity check — exercises LoadConfig end-to-end.
go run . consumption  # Aggregate api-consumption-report output across the fleet (read-only).
go run . template fetch   # ~3 min (serial); do not run in subagents with strict timeouts.
```

For third-party API shape or documentation questions (e.g. zerolog field-writer signatures, cobra persistent-flag ordering), prefer `go doc <pkg>` / `go doc <pkg>.<Symbol>` over web search — it returns the exact API surface for the pinned version in `go.mod`. When editing, `gopls` (running via the IDE or `gopls definition`/`gopls symbols`) resolves identifiers against the current module state, which catches rename drift faster than grep.

### Local gate — run before claiming any code change complete

`go build` / `go vet` are necessary but **not sufficient**. CI runs stricter checks (gofmt alignment, golangci-lint, full test suite) that build+vet do not. Before reporting any code change "done," run the full gate:

```bash
make fmt         # apply gofmt in place (or `make fmt-check` to verify without writing)
make lint        # golangci-lint — can exceed 5 minutes; use extended timeout, do NOT skip
make test        # full test suite
# — or, in one shot: —
make ci          # runs: fmt-check vet lint test  (this is the CI gate)
```

**Do not claim a task complete until `make ci` passes locally.** `go build` passing means nothing — prior commits have landed lint/fmt failures that CI rejected because only build+vet were run. When in doubt, run `make ci`.

**Toolchain note:** the current local development gate runs unpinned with Go 1.26.4 and golangci-lint v2.12.2. Run `make ci` directly; do not set `GOTOOLCHAIN=go1.25.8`. If local results look suspicious, confirm `go version` and `golangci-lint --version` before trusting the gate. The `go.mod` directive records module compatibility and is not a reason to downshift the local lint/test toolchain.

The CLI has two config files: `fleet.json` (public, committed, example tracking only `rshade/gh-aw-fleet`) and `fleet.local.json` (private, gitignored, real fleet state). `LoadConfig` reads `fleet.json` as the **base** and overlays `fleet.local.json` on top — when both exist they are merged (local profiles/repos/defaults add to or override base entries); when only one exists it is used directly. `go run . list` prints `(loaded fleet.json + fleet.local.json)`, `(loaded fleet.json)`, or `(loaded fleet.local.json)` to stderr so you know which mode is active.

## Architecture big-picture

`gh-aw-fleet` is a **thin orchestrator** around three upstream tools: `gh aw` (the markdown→GitHub Actions compiler), `gh` (PR creation), and `git` (branching/commits). It never rewrites workflow markdown. All value is in *which repo gets which profile, when, pinned to which ref* — not in file manipulation. When adding features, prefer shelling out to upstream tools over re-implementing their logic.

**Declarative reconcile loop**: `fleet.json` declares desired state; `deploy`/`sync`/`upgrade` compute diffs and apply. Every command is dry-run by default and requires `--apply` for destructive operations.

**Pin-per-profile, keyed by source repo**: Each profile has a `sources` map (e.g., `github/gh-aw` → `v0.79.2`, `githubnext/agentics` → `main`). Bumping one source ref re-pins every workflow in that profile from that source. Profile definitions in `fleet.json` must match `profiles/default.json` verbatim for the `default` profile.

**Framing that shapes everything**: `github/gh-aw` ships the compiler + dogfooding tests; `githubnext/agentics` ships the curated library. When adding workflows to profiles, prefer agentics unless no equivalent exists. `github/gh-aw`'s `main` often contains unreleased features (like `mount-as-clis`) that break the installed CLI — always pin gh-aw sources to tags, never to `main`.

**`*-plus` profiles are opt-in, cost-aware bundles**: `quality-plus`, `security-plus`, `docs-plus`, `community-plus`, and `observability-plus` are layered onto `default` per-repo via the repo's `profiles` list. `observability-plus` ships `api-consumption-report` (daily) — pair it with `gh-aw-fleet consumption` (see below) for fleet-wide billing rollups, but note that opting a repo in incurs recurring Copilot-credit cost since the report is itself an LLM workflow.

**Billing-metadata fields are advisory, not enforced**: `Profile.Tier` (`tier` on a profile, recommended `minimal | standard | premium`) and `RepoSpec.CostCenter` (`cost_center` on a repo, free-form) are optional declarative annotations the loader silently accepts when absent. Neither bumps `fleet.SchemaVersion` or the `--output json` envelope's `schema_version` — both are additive. Both are group-by keys for `gh-aw-fleet consumption` (see below). The tool does not validate values, gate behavior on them, or restrict which file (`fleet.json` vs `fleet.local.json`) they appear in — operator self-polices the public/private split. Surfaced in `gh-aw-fleet list` as a parallel `TIERS` column and an appended `COST_CENTER` column; see godoc on the `Profile` and `RepoSpec` types for the field-level contract.

**Consumption rollup**: `gh-aw-fleet consumption` is a read-only fleet-wide AI-credit (AIC) aggregator. `--source` selects the data path (default `logs`):

- **`--source logs` (default, issue #103)**: per repo, enumerate the agentic workflows via `gh api repos/.../actions/workflows` (those compiled to `.lock.yml`), then call `gh aw logs --json "<display name>"` per workflow and sum `summary.total_aic` (+ per-run `runs[].aic`). **Needs no deployed `api-consumption-report` workflow** — the rollup is decoupled from the deploy. Two gotchas baked into the impl: `gh aw logs <WORKFLOW>` requires the GitHub Actions **display name** ("Daily Malicious Code Scan Agent"), not the fleet slug (`daily-malicious-code-scan`), so the fan-out sources names from the Actions API; and an **unqualified** `gh aw logs --repo X` returns zero runs, so it must be scoped per workflow. `gh aw logs` writes artifacts under `./.github/aw/logs`, so `ghLogsAPI` runs each call in a throwaway temp dir. Seams: `ghWorkflowsAPI`, `ghLogsAPI` (`internal/fleet/consumption_logs.go`).
- **`--source artifacts` (transitional fallback)**: the legacy path — discovery via `gh api repos/.../discussions` filtered to the `audits` category + the `<!-- gh-aw-tracker-id: api-consumption-report-daily -->` marker; data via `gh api repos/.../actions/runs/{run_id}/artifacts` reading `aw_info.json` + `run_summary.json`. Seams: `ghDiscussionsAPI`, `ghRunArtifactAPI`. Note: real `aw_info.json` carries **no** USD `cost` field under Copilot AI-Credits, so this path's COST/AIC columns are usually empty (it predates the AIC schema).

**AIC is the metric; USD is derived as `AIC * 0.01`** (`aicToUSDRate`). Both render as columns (`AIC`, `COST`); both honor the nil-until-positive rule and the all-or-nothing group-merge (a repo with only failed runs — `aic` is absent on failures — rolls up to nil, triggering `nilAICDiag`). Three mutually-exclusive temporal modes — `--latest` (default), `--trailing Nd`, `--since YYYY-MM-DD` — map to `gh aw logs` `--start-date`/`-c` with a deterministic client-side `created_at` re-filter. Four group-by axes — `--by repo|profile|cost-center|workflow`; multi-profile repos contribute additively to every profile group (Decision 5); repos with no `cost_center` land under the literal `<unset>` bucket. No caching (FR-022). All `gh`/`gh api` paths go through package-level injection seams so tests run offline against `internal/fleet/testdata/{consumption,logs}/` fixtures. The `AIC`/`Source` envelope fields are additive — no `cmd.SchemaVersion` bump.

**Critical asymmetry**: `gh aw add` uses `fleet.json` pins (via `ResolvedWorkflow.Spec()`). `gh aw update` follows the workflow's *own* frontmatter `source:` line, not `fleet.json`. This means fleet.json edits don't propagate through `upgrade` — they need a `fleet sync --apply --force <repo>` to re-pin workflows to current fleet.json refs. (Init artifacts and the fleet manifest are a separate axis: `upgrade` *does* refresh those to the fleet's `github/gh-aw` pin — see Deploy-quirks (1)/(4) — only the per-workflow `source:` refs follow frontmatter.)

**`gh aw` path conventions differ by source**: agentics uses 3-part specs (`githubnext/agentics/<name>@ref`, implicit `workflows/` dir); gh-aw needs 4-part (`github/gh-aw/.github/workflows/<name>.md@ref`). `internal/fleet/fetch.go`'s `SourceLayout` map encodes this; `ResolvedWorkflow.Spec()` consumes it. Adding a third source means adding a `SourceLayout` entry.

**Diagnostic layer**: `internal/fleet/diagnostics.go` defines `CollectHints(texts...)` which scans error output for known patterns (unknown-property, HTTP 404, gpg failure) and returns actionable remediations. `cmd/deploy.go` and `cmd/upgrade.go` surface hints in failure paths. When a new class of error shows up, add a hint entry — it's a single-file edit.

**Deploy absorbs gh-aw CLI quirks**: `internal/fleet/deploy.go` works around four upstream `gh aw` behaviors so a `deploy` is reliable across CLI versions — (1) after `gh aw init` runs, `ensureInit` asserts that at least one recognized init marker exists (the `initMarkers` list — e.g. `.github/agents/agentic-workflows.md`, `.github/skills/agentic-workflows/SKILL.md`, observed from `gh aw init` at the v0.79.2 pin) and emits a non-fatal `init_no_recognized_marker` warning when none do, catching an upstream init-layout change out from under the hardcoded list; the *skip* decision itself is the manifest-version comparison in (4), not a marker file-check; (2) `fixMisplacedSkillImports` relocates a mis-nested `.github/workflows/.github/` skill tree up to `.github/` (a `gh aw add` path bug that drops skill imports one dir too deep) then recompiles the added workflows; (3) `recompileAddedWorkflows` **must forward `--engine`** — `gh aw add --engine X` overrides the engine only for *its own* compile and never rewrites the workflow `.md`, so any *later* `gh aw compile` reverts to the markdown's native engine (e.g. `engine: claude`) and the deployed lock then demands the wrong secret (`ANTHROPIC_API_KEY`) at runtime; (4) `ensureInit` now uses a manifest version comparison instead of a file-presence check (`initMarkerPaths` has been removed). When a clone contains `.github/aw/fleet-manifest.json` with a `gh_aw_version` that matches the fleet's current `github/gh-aw` source pin, init is skipped; otherwise `gh aw init` runs. This handles legacy repos that were initialized by older gh-aw versions — version mismatch triggers a refresh. `deploy`, `sync`, and `upgrade` all run `ensureInit` and write the manifest, so an `upgrade` on a repo whose init predates a pin bump refreshes the dispatcher/init layout (recording the new version in the PR) instead of recompiling lock files against a stale layout. **Keep the local `gh aw` CLI version matched to the fleet's source pins** (`fleet.json` refs) — a mismatch breaks `init`/`add`/`compile` in version-specific ways (the CLI both generates and `--strict`-validates lock files, so a skew can make it reject its own output).

**HuJson on the read/write paths**: `internal/fleet/load.go` runs every config file (`fleet.json`, `fleet.local.json`, `templates.json`) through `hujson.Standardize()` before `json.Unmarshal`, so `//` line comments, `/* */` block comments, and trailing commas are accepted everywhere. The loader probes `<base>.hujson` first and falls back to `<base>.json`; both files present is rejected as ambiguous. Writes preserve operator-authored comments via direct AST mutation (`Add` appends to `/repos`) or RFC 6902 `Patch` (`SaveTemplates` replaces `/version` + `/fetched_at` + `/sources` and leaves `/evaluations` untouched). Only `fleet.local.json` and `templates.json` are written; `fleet.json` and `profiles/default.json` are read-only. When a comment-preserving write fails, callers fall back to `writeJSON` and emit `level=warn, event=hujson_fallback_to_rewrite`.

## Hard invariants

- **Never bypass gpg signing.** Don't add `--no-gpg-sign`, `-c commit.gpgsign=false`, or `git config commit.gpgsign false` to any command. The tool lets gpg fail; the user finishes manually in their shell. `.claude/settings.json` denies these at the allowlist level.
- **Never run `git add`, `git commit`, `git push` from the Bash tool.** The Go tool invokes these via `exec.Command` inside `Deploy`/`Upgrade`/`Sync` — that's the one legitimate path. Claude never invokes git directly. `.claude/settings.json` deny list enforces this.
- **Commit messages and PR titles use Conventional Commits with `ci(workflows)` scope.** See `commitMessage()` / `upgradeTitle()` in `internal/fleet/`. Subject ≤72 chars, no trailing period, type is always `ci` (these ARE CI configuration changes).
- **Never hand-edit `CHANGELOG.md`.** Release-please manages it: on every push to `main`, the action in `.github/workflows/release-please.yml` opens a release PR that prepends a `## [version]` section generated from conventional commits. The type→section map lives in `release-please-config.json` (`feat:` → Added, `fix:` → Fixed, `refactor:` → Changed, `perf:` → Performance, `docs:` → Documentation; `chore:`/`ci:`/`build:`/`test:`/`style:` are hidden). The **commit message IS the changelog entry** — write the subject accordingly. Manual edits to version sections will be overwritten. This overrides the global CLAUDE.md rule "always update CHANGELOG.md." (The stale `## Unreleased` block at the top is a pre-automation artifact, not a pattern to extend.)
- **Clone dirs at `/tmp/gh-aw-fleet-*` are breadcrumbs after failure** — the tool preserves them when `--apply` fails mid-pipeline. Don't delete them; the user inspects / resumes from them.
- **`--work-dir` resumes a prior run.** Deploy's commit gate checks `hasStagedOrUnstagedWorkflowChanges` so a previously-interrupted `--apply` completes correctly when re-invoked with `--work-dir <clone>`.
- **`.github/aw/fleet-manifest.json` is written by the fleet tool only.** Do not hand-edit it; its content drives the version-drift detection and `ensureInit` skip logic.

## Code self-documentation

- **Every exported identifier gets a godoc comment.** Package, type, function, method, const, var, and exported struct field. Format: one complete sentence starting with the identifier name and ending with a period (`// Config is the declarative desired state for the fleet.`). For `Opts` / `Result` structs, document field meaning inline (`Force bool // pass --force to gh aw add`) — mirror the `DeployOpts` / `DeployResult` pattern in `internal/fleet/deploy.go`.
- **One `// Package foo` comment per package** on any file. Keep it short (1–3 sentences describing scope, not a feature tour).
- **Prefer expressive names over comments for unexported code.** If an internal helper needs a comment to explain *what* it does, rename it. Comments on unexported code should explain *why* — constraints, invariants, non-obvious tradeoffs — not restate the code.
- **`make lint` enforces this** via `revive` and `staticcheck`. Do **not** re-add the `should have a package comment` / `exported X should have comment` suppressions to `.golangci.yml` — they were removed deliberately. If a check fires against a legitimate exception, narrow the suppression to that path, don't blanket-disable the rule.
- **Two `SchemaVersion` constants live in this codebase.** `cmd.SchemaVersion` versions the JSON output envelope (wire contract); `fleet.SchemaVersion` versions the on-disk fleet.json format. They bump independently. Godoc on both must state which they are — the linter won't catch that they mean different things.

## Testing deploys requires care

`--apply` pushes a branch and opens a PR on an **external** repository. In interactive sessions, always get explicit user go-ahead ("go", "apply", "yes") in the turn before running `--apply`. In subagent test contexts, hard-stop instructions must forbid `--apply` — dry-runs are the test surface.

## Skills

The `skills/` directory contains six SKILL.md files codifying recurring operator workflows: `fleet-deploy`, `fleet-eval-templates`, `fleet-upgrade-review`, `fleet-onboard-repo`, `fleet-build-profile`, and `fleet-budget-review`. The first four encode the three-turn pattern (dry-run → user approval → apply) and the gpg-failure manual-finish paste template. `fleet-build-profile` picks up where `fleet-eval-templates` stops — the latter evaluates which upstream workflows deserve inclusion, the former materializes a chosen set as a `profiles.<name>` entry in `fleet.json` (and, for the `default` profile, the mirrored `profiles/default.json`). `fleet-budget-review` is the read-only consumption-rollup counterpart — single-turn flow over `gh-aw-fleet consumption` (no mutation, so no three-turn pattern), framing cost concentration by repo / profile / cost-center / workflow. When adding features that affect these flows, update the relevant SKILL.md alongside the code.

## .claude/settings.json

Committed at repo root; shared with collaborators and subagents. Allows `go build/vet/test/run`, `gh aw/api/repo/pr`, `git` read ops, and common shell tools. Denies `git add`, `git commit`, `git rebase --continue`, `git push --force`, `git reset --hard`. When adding a new developer command, add it to the allowlist so subagents don't prompt.

## Active Technologies
- Go 1.26.4 for the local development gate (with `go.mod` currently declaring module compatibility at `go 1.25.8`), using `github.com/spf13/cobra` v1.10.2 for CLI wiring and `github.com/rs/zerolog` v1.x for structured logging on stderr.
- `fleet.local.json` is the private, gitignored source of truth; `fleet.json` is the committed public example.
- Structured logging: `internal/log.Configure(level, format)` wires a zerolog global logger in root's `PersistentPreRunE`; warnings/errors/subprocess summaries emit on stderr, tabwriter status stays on stdout.
- Go 1.26.4 local toolchain. + `encoding/json` (stdlib, new usage site); `github.com/spf13/cobra` v1.10.2 (existing); `github.com/rs/zerolog` v1.35.1 (existing, landed in #34). No new third-party dependencies — constitution Principle I. (main)
- N/A (no persistent state; envelope writes are transient to stdout). (main)
- Go 1.26.4 local toolchain. + `github.com/spf13/cobra` v1.10.2 (CLI), `github.com/rs/zerolog` v1.x (stderr structured logging), `gopkg.in/yaml.v3` (frontmatter parsing — already in use), `encoding/json` (stdlib, JSON envelope). **No new third-party dependencies** (SC-006 / Constitution Principle I). (004-status-drift-detection)
- N/A — pure read command, no on-disk state, no cache. Output is transient to stdout. (004-status-drift-detection)
- Go 1.26.4 local toolchain. + `github.com/spf13/cobra` v1.10.2 (CLI), `github.com/rs/zerolog` v1.x (stderr structured logging), `encoding/json` (stdlib, JSON envelope). **No new third-party dependencies** (Constitution Principle I; Assumptions in spec). (005-actions-preflight)
- N/A — pure read calls to the GitHub API; no on-disk state, no cache. Findings are transient on `DeployResult`. (005-actions-preflight)
- Go 1.26.4 local toolchain. + `github.com/tailscale/hujson` (BSD-3-Clause, zero transitive deps) for comment-preserving reads/writes of `fleet.json`, `fleet.local.json`, `templates.json`, and `profiles/default.json`. Approved direct dependency under [Constitution v1.1.0 §Third-Party Dependencies](./.specify/memory/constitution.md). (issue #73)
- N/A — read path runs `hujson.Standardize()` before `json.Unmarshal`; write path uses direct AST mutation (`hujson.Parse`/`Pack`) for `Add` and RFC 6902 patches (`hujson.Patch`) for `SaveTemplates`. Probes `<base>.hujson` then `<base>.json`; both present is rejected as ambiguous. (issue #73)
- Go 1.26.4 local toolchain. + `github.com/spf13/cobra` v1.10.2 (CLI), `github.com/rs/zerolog` v1.35.1 (stderr structured logging), `archive/zip` + `encoding/json` (stdlib, in-memory artifact unzip + decode), `regexp` (stdlib, body-marker parsing). **No new third-party dependencies** (Constitution Principle I). (009-consumption-subcommand)
- N/A — pure read calls against `gh api` discussions + `gh api` run artifacts; no on-disk state, no cache, no persisted index (FR-022). Output is transient to stdout. (009-consumption-subcommand)
- Go 1.26.4 local toolchain. + `github.com/tailscale/hujson` (existing approved direct dep — JWCC-tolerant parse of Renovate configs); stdlib `encoding/json`, `os`, `path/filepath`, `strings`. **No new third-party dependencies** (Constitution Principle I). (012-renovate-conflict-scanner)
- N/A — pure read of a probed Renovate config in the work-dir clone; no on-disk state, no cache. Findings are transient on `DeployResult`/`SyncResult`/`UpgradeResult`. (012-renovate-conflict-scanner)

## Recent Changes
- 012-renovate-conflict-scanner: a fourth read-only advisory scanner in the slice-006 security registry (`internal/fleet/security/renovate.go`, wired in `defaultScanners()`) — warns at `LOW` when a repo's Renovate config does not disable `gh-aw-actions` bumps or exclude `.github/workflows/*.lock.yml`; unparseable config → one `INFO`; new `security_renovate` diag code (one family code, per-rule granularity on `Fields["rule_id"]`); no new deps, no schema-version bump; surfaces on `deploy`/`sync`/`upgrade` via the existing finding pipeline.
- 009-consumption-subcommand: new read-only `gh-aw-fleet consumption` subcommand aggregates `api-consumption-report` output across the fleet; three temporal modes (`--latest`/`--trailing Nd`/`--since YYYY-MM-DD`) and four group-by axes (`--by repo|profile|cost-center|workflow`); no new third-party deps; reuses existing diagnostic codes (`DiagHint`, `DiagHTTP404`).
- issue-73 (hujson): comment-preserving config — `//` line, `/* */` block, and trailing-comma syntax accepted in `fleet.json` / `fleet.local.json` / `templates.json` / `profiles/default.json`; `.hujson` extension preferred over `.json`; `gh-aw-fleet add` no longer overwrites prior repo entries when the local file exists.
- 002-add-zerolog-logging: added `--log-level` / `--log-format` persistent flags; `⚠ WARNING:` lines in `deploy`/`sync` moved to stderr as structured `warn` events; subprocess summaries at `debug`.
- 001-add-subcommand: added `gh-aw-fleet add <owner/repo>` subcommand (cobra) for onboarding repos into `fleet.local.json`.
