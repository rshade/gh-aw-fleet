# Implementation Plan: Fleet Overview Subcommand

**Branch**: `018-overview-subcommand` | **Date**: 2026-06-23 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/018-overview-subcommand/spec.md`

## Summary

Add a new read-only subcommand `gh-aw-fleet overview` that renders one consolidated cross-repo dashboard joining three signals per repo into a single row: **drift** (from `fleet.Status()`), **run health** (RUNS / FAIL / HEALTH derived from `gh aw logs --json` `.runs[].conclusion`), and **cost + run-rate** (AIC / COST / NOOP from the same `gh aw logs` payload). Columns: `REPO | DRIFT | RUNS | FAIL | NOOP | HEALTH | AIC | COST`, closed by a pooled `TOTAL` row, with a per-repo detail block for any drifted/errored/unhealthy repo.

It reuses the two API paths the fleet already has — `fleet.Status()` for drift and the `ghWorkflowsAPI` → `ghLogsAPI` → `filterRunsByWindow` fan-out for health+cost — and does **not** re-implement either. The slow per-repo fan-out runs concurrently with the drift query (two top-level batches). Three mutually-exclusive temporal selectors (`--latest` / `--trailing <Nd>` / `--since <YYYY-MM-DD>`) are reused from `consumption`, but the **default is `--trailing 7d`** (health is a windowed concept). Positional `[repo...]` scopes the dashboard via `ScopeToRepos`.

Two design points are settled by the spec and grounded in live-fleet investigation:

1. **Health taxonomy is failures-only** (FR-007): `success` → healthy; `failure`/`timed_out`/`startup_failure` → failure; `cancelled`/`skipped`/non-terminal → excluded from the denominator. So `HEALTH = successes ÷ RUNS` where `RUNS = successes + failures`.
2. **No-op detection is an aggregate, not per-run** (FR-031, empirically verified against `rshade/finfocus`): `gh aw logs --json` v0.79.2 exposes **no** per-run no-op flag, but `mcp_tool_usage.summary[]` carries `{server_name: "safeoutputs", tool_name: "noop", call_count: N}` — which matched the success-run count exactly (9 ↔ 9) in testing. `NOOP` per repo is the summed `noop` `call_count`, clamped to `[0, successes]`. This requires extending `logsPayload` to decode `mcp_tool_usage` (currently dropped on unmarshal — additive).

Exit code mirrors `status`: non-zero iff any in-scope repo is drifted/errored; run failures are advisory (FR-018/FR-019). JSON via `writeEnvelope` with a new `OverviewResult` payload; `cmd.SchemaVersion` stays `1` (additive). No new third-party dependencies.

## Technical Context

**Language/Version**: Go 1.26.4 (per `go.mod` `go 1.26.4` directive).
**Primary Dependencies**: `github.com/spf13/cobra` v1.10.2 (CLI), `github.com/rs/zerolog` v1.35.1 (stderr structured logging via `internal/log`), `encoding/json` / `time` / `os/exec` (stdlib, the last via the existing `ghLogsAPI`/`ghWorkflowsAPI` seams). **No new third-party dependencies** — within the approved set under Constitution v1.3.0 § Third-Party Dependencies. `ax-go` `config`/`schema` are in the tree but not needed here.
**Storage**: N/A. Pure read calls against `gh api` (workflows list, contents for drift) and `gh aw logs --json`. No on-disk state, no cache, no persisted baseline (FR-026). Output transient to stdout; diagnostics to the envelope + stderr.
**Testing**: `go test ./internal/fleet/... ./cmd/...` with the existing co-located `*_test.go` pattern; `make ci` (`fmt-check vet lint test`) is the local gate. Every external call goes through the existing package-level injection seams (`ghWorkflowsAPI`, `ghLogsAPI`, and `Status`'s `statusFetcher`) so all tests run offline against fixtures (SC-006). No live network in CI.
**Target Platform**: Linux/macOS/Windows single CLI binary; `go build ./...` clean across platforms.
**Project Type**: Single-binary Go CLI (`cmd/` cobra wiring, `internal/fleet/` business logic). No new packages.
**Performance Goals**: Drift batch and health/cost batch run **concurrently** (two goroutines); within each, per-repo work follows the existing pattern (`Status` already parallelizes per repo internally; the logs fan-out is serial per repo/workflow today). Worst case for a typical fleet (≤10 repos, default 7-day window) stays within Constitution Principle IV's 5-minute ceiling; the `gh aw logs` fan-out is the dominant cost and downloads artifacts (observed). Bounded-concurrency / no-download fast path (#113) and discovery pagination (#119) are **consumed when available, not re-solved here** (Assumptions); the wall-clock cost is documented in `--help`.
**Constraints**: `cmd.SchemaVersion = 1` MUST NOT bump (SC-004). Read-only — no `--apply`, no git, no gpg, no writes (FR-002). Failures-only health taxonomy (FR-007). NOOP is a window-aggregate, not client-side re-filterable per run — clamp to `[0, successes]`; for `--latest` (`-c 5`, no `--start-date`) the aggregate-vs-kept-run mismatch is a documented best-effort limitation (research.md §3).
**Scale/Scope**: New `internal/fleet/overview.go` (+ `overview_test.go`); new `cmd/overview.go` (+ `overview_test.go`); one-line `cmd/root.go` registration; one new `commandOverview` const in `cmd/output.go`; additive `mcp_tool_usage` decode on `logsPayload` in `internal/fleet/consumption_logs.go` plus a shared per-repo run-extraction helper factored out of `logSourceToReports` (behavior-preserving, guarded by existing consumption tests). Fixtures: reuse `internal/fleet/testdata/logs/` + `status` fixtures; add a `mixed health + noop` logs fixture. Docs: README, `docs/src/content/docs/overview.md` (new, mirrors `consumption.md`), AGENTS.md/CLAUDE.md; optionally extend the `fleet-budget-review` skill (defer to tasks.md).

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Per Constitution v1.3.0:

- **I. Thin-Orchestrator Code Quality**. PASS. Overview composes two existing orchestration paths — `fleet.Status()` (which wraps `gh api` contents) and the `gh aw logs` fan-out — and joins their results. No new external-tool logic; no re-implementation of drift detection or the logs fan-out (FR-003/FR-004). The one new decode (`mcp_tool_usage`) reads data `gh aw logs` already emits. Exported identifiers get full godoc per CLAUDE.md "Code self-documentation". The new files stay focused; `overview.go` holds the join + types, well under the 300-line guidance. `go build`/`go vet` clean is the bar.
- **II. Testing Standards**. PASS. Read-only command — no `--apply`, so no dry-run obligation under Principle II.2. Unit tests cover: (a) conclusion → health classification (all-pass / all-fail / mixed / cancelled-skipped-excluded / empty); (b) NOOP aggregate extraction + clamp; (c) the drift × health × cost join, including per-signal isolation (drift errored but health fetched, and vice versa); (d) temporal-flag parse + mutual exclusion + `--trailing 7d` default; (e) exit-code matrix (aligned→0, drifted→1, errored→1, failures-but-aligned→0); (f) JSON envelope shape (mirrors `cmd/status_test.go` / `list_result_test.go`). Injection seams keep every test offline. `make ci` is the gate.
- **III. UX Consistency (Three-Turn Mutation Pattern)**. PASS BY EXEMPTION. The three-turn pattern governs mutating commands. `overview` is read-only (FR-002), like `list`, `status`, and `consumption`. Recoverable per-repo failures still route through `fleet.Diagnostic` so the operator sees actionable context (FR-024).
- **IV. Performance**. PASS. Drift and health/cost batches run concurrently (Principle IV "SHOULD parallelize I/O-bound independent targets"). **No caching** — the dashboard is per-invocation operational state where staleness is the failure mode, the documented exception class (FR-026; same reasoning as `consumption` FR-022). The 5-minute ceiling holds for a typical fleet on the default window; the `gh aw logs` artifact-download cost is inherited from the reused fan-out and is documented in `--help`, with #113/#119 noted as the upstream improvements to consume.
- **Declarative Reconcile Invariants**. PASS. Reads `fleet.json`/`fleet.local.json` via the existing `LoadConfig`; no schema additions, no writes. gpg/`git` restrictions don't apply to a read command. No source-pin changes. `ScopeToRepos` returns a restricted copy; the base config is untouched.
- **Third-Party Dependencies**. PASS. **No additions to `go.mod`'s `require()` block.** `encoding/json`, `time`, `os/exec` (stdlib via existing seams); reuses approved `cobra`/`zerolog`. No amendment required.
- **Development Workflow**. PASS. PR will include `make ci` evidence and a fixture-driven `go run . overview --output json` sample. Documentation obligations recorded in the Documentation Impact gate below.

**No violations to justify.** Complexity Tracking section stays empty.

## Documentation Impact

*GATE: Per Constitution v1.3.0 Development Workflow — `README.md` and the `docs/` Starlight site MUST be updated in the same change that alters a surface they document.*

- **Surfaces touched**: README (new `overview` command in the command list + a short usage block); `docs/src/content/docs/overview.md` (NEW page, mirroring `consumption.md` — columns, temporal flags, drift-only exit code, advisory run failures, the NOOP/run-rate column, wall-clock note); `docs/src/content/docs/index.mdx` and the Starlight sidebar (add the overview page); AGENTS.md / CLAUDE.md ("Common commands" line + an architecture paragraph per FR-027). Optionally the `fleet-budget-review` skill gains an overview cross-reference (defer to tasks.md).
- **Update planned**: yes — `overview` is a new user-facing command; README + the docs site + the agent files are all in scope this slice (FR-027 + constitution docs-currency rule).
- **Hidden surfaces**: none. (Overview is fully public; contrast the deliberately-hidden `__schema` command.)

## Project Structure

### Documentation (this feature)

```text
specs/018-overview-subcommand/
├── plan.md              # This file (/speckit-plan command output)
├── spec.md              # Feature spec (/speckit-specify)
├── research.md          # Phase 0 output (/speckit-plan)
├── data-model.md        # Phase 1 output (/speckit-plan)
├── quickstart.md        # Phase 1 output (/speckit-plan)
├── contracts/
│   ├── overview-output.json        # Phase 1: JSON envelope + OverviewResult shape
│   ├── overview-text-output.md     # Phase 1: tabwriter columns + TOTAL + detail blocks
│   └── run-health-extraction.md    # Phase 1: conclusion taxonomy + noop aggregate + windowing caveat
├── checklists/
│   └── requirements.md  # Spec quality checklist (/speckit-specify)
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

Single-binary Go CLI; `*_test.go` lives next to the code under test (no separate `tests/` tree). Files this feature touches:

```text
internal/
├── fleet/
│   ├── overview.go               # NEW: Overview(ctx, cfg, OverviewOpts) (*OverviewResult, []Diagnostic, error);
│   │                             #      OverviewOpts / OverviewResult / RepoOverview / OverviewTotal types;
│   │                             #      the drift×health×cost join; classifyConclusion; health/noop reducer.
│   ├── overview_test.go          # NEW: table tests for classification, noop clamp, join, isolation,
│   │                             #      exit-disposition; envelope-shape assertions.
│   ├── consumption_logs.go       # EDIT (additive): add mcpToolUsage decode to logsPayload; factor a shared
│   │                             #      per-repo run-extraction helper (e.g. collectRepoRuns) out of
│   │                             #      logSourceToReports so overview reuses the SAME fan-out + window filter.
│   └── testdata/logs/
│       └── logs_mixed_noop.json  # NEW fixture: mixed success/failure/cancelled + mcp_tool_usage noop count.
cmd/
├── overview.go                   # NEW: cobra subcommand; mirrors consumption.go (temporal flags + mutual
│                                 #      exclusion, ScopeToRepos, FetchMode build w/ trailing-7d default),
│                                 #      mirrors status.go (printRepoDetail-style blocks + drift exit code).
├── overview_test.go              # NEW: flag parse/default/exclusion, text + JSON render, exit-code matrix.
├── root.go                       # ONE-LINE EDIT: root.AddCommand(newOverviewCmd(&flagDir))
└── output.go                     # ONE-LINE EDIT: add `commandOverview = "overview"` const.
                                  #   (overview does NOT call rejectJSONMode → JSON supported by default.)

# Documentation (FR-027 + Constitution v1.3.0):
README.md                          # command list + usage block
docs/src/content/docs/overview.md  # NEW page (mirrors consumption.md)
docs/src/content/docs/index.mdx    # + Starlight sidebar entry
AGENTS.md                          # architecture paragraph + "Common commands" line
CLAUDE.md                          # SPECKIT marker repoint (done in this plan); mirrors AGENTS.md
```

**Structure Decision**: Single-binary Go CLI, layout already established (`cmd/` + `internal/fleet/`). No new packages. `overview.go` holds the orchestrator, the result types, and the join — consistent with peer features (`status.go`, `consumption.go`). The only shared-code change is in `consumption_logs.go`: extend `logsPayload` to decode `mcp_tool_usage` and extract a shared per-repo run loop from `logSourceToReports`, so both `consumption` and `overview` consume one fan-out (the "factor the fan-out" the spec calls for). The refactor is behavior-preserving and guarded by the existing `consumption_test.go` suite.

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

*No violations. Section intentionally empty.*
