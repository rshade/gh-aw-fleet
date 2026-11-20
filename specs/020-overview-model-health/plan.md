# Implementation Plan: Flag Repos Failing on Unsupported/Deprecated Copilot Models in `overview`

**Branch**: `020-overview-model-health` | **Date**: 2026-07-08 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/020-overview-model-health/spec.md`

## Summary

`overview` already fans out per-repo drift (`Status`) and run-health (`collectRepoRuns` → `gh aw logs`) concurrently and joins them into `RepoOverview` rows. Today it cannot tell a *model-not-supported* failure apart from any other failure, because the decoded `logsRun` struct carries only `conclusion`/`aic`/counts — no failure category.

This feature adds a **third concurrent collector**, `collectOverviewModelHealth`, that reads each repo's auto-filed **`agentic-workflows` failure issues** and matches the machine-readable marker `<!-- gh-aw-failure-issue: true, … failure_categories: …|model_not_supported_error -->`. A repo with any open failure issue in that category is flagged. For each flagged repo — and only for flagged repos — it resolves the **effective Copilot model** from repo Actions variables (`GH_AW_MODEL_AGENT_COPILOT || GH_AW_DEFAULT_MODEL_COPILOT || <gh-aw baked default>`) to name the offending model. The flag renders as a line in the affected repo's per-repo detail block (stdout), a remediation hint through the existing hints channel (stderr) naming `GH_AW_DEFAULT_MODEL_COPILOT`, and an additive `model_support` pointer field on `RepoOverview` in `--output json`. Exit code stays drift-gated; the advisory never changes it. Detection reuses surfaces reachable via `gh`; no new third-party dependency, no `cmd.SchemaVersion` bump.

## Technical Context

**Language/Version**: Go 1.26.4 (local development gate; `go.mod` directive `go 1.26.4`).
**Primary Dependencies**: Existing only — `github.com/spf13/cobra` v1.10.2 (CLI), `github.com/rs/zerolog` v1.35.1 (stderr warnings/hints); stdlib `context`, `encoding/json`, `os/exec`, `regexp`, `sort`, `strings`, `sync`. **No new third-party dependencies** (Constitution §Third-Party Dependencies / spec Assumptions).
**Storage**: N/A — pure read dashboard. No cache, no baseline, no on-disk state. Model-support status is transient on the `RepoOverview` row, consistent with AIC/Cost.
**Testing**: `go test ./...` with offline fixtures. New package-level seams (`ghFailureIssuesAPI`, `ghRepoVariablesAPI`) are injected in tests exactly as `ghWorkflowsAPI`/`ghLogsAPI` are today via `withLogsStubs`; failure-issue body fixtures live under `internal/fleet/testdata/failures/`.
**Target Platform**: Linux/macOS developer + CI shells with an authenticated `gh` session.
**Project Type**: Single Go CLI module (thin orchestrator over `gh aw`/`gh`/`git`).
**Performance Goals**: Stay under the constitution's 5-minute ceiling for a typical fleet (≤10 repos). The failure-issue collector runs concurrently with the existing drift + logs collectors (a third `wg.Go`); `gh variable list` runs only for flagged repos (rare), so it adds at most one call per already-broken repo.
**Constraints**: Read-only; additive JSON (no `cmd.SchemaVersion` bump, no field renamed/removed); exit code gated on drift only; a fully healthy, drift-aligned fleet's output stays byte-identical (SC-003); deterministic, fixture-testable ordering (FR-012).
**Scale/Scope**: Fleet scope = the scoped `cfg.Repos` (honors positional repo args). One new collector, one new result entity (`ModelSupport`), two new `gh` seams, one new baked-default constant, renderer + hint additions in `cmd/overview.go`.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **I. Thin-Orchestrator Code Quality** — PASS. Detection wraps `gh issue list` and `gh variable list` via `exec.Command` behind package-level seams; it parses upstream-authored markers rather than re-implementing any `gh aw` logic. New code stays in the existing `internal/fleet/overview.go` + `cmd/overview.go` surfaces; `overview.go` is ~416 lines and the added collector keeps it near that budget (extract to `overview_model.go` if it drifts materially past 300 lines with the new logic — see Structure Decision). Errors wrap with `%w`; exported identifiers get godoc.
- **II. Testing Standards** — PASS. `overview` is a read-only command (no `--apply`, no mutation), so the dry-run-against-real-clone rule does not apply. The build-green gate (`go build`/`go vet`) plus `make ci` (fmt, lint, full test suite) with fixtures covering model-failure / unrelated-failure / healthy / undetermined repos (SC-006) is the bar. No skill changes are load-bearing here (see Documentation Impact).
- **III. User Experience Consistency** — PASS (with a noted exception). The three-turn mutation pattern does not apply — `overview` mutates nothing. The remediation routes through the existing hints channel (`fleet.CollectHints`-style `DiagHint` diagnostics surfaced by `emitOverviewDiagnostics`), satisfying the "every recoverable failure routes through a hint" spirit. No commit/PR is produced.
- **IV. Performance Requirements** — PASS. The new collector is parallelized with drift and health via `wg.Go`; per-repo failure-issue reads happen inside that goroutine. No network-derived catalog state is cached today by `overview` and none is added. The `gh variable list` call is gated to flagged repos, minimizing added I/O.

**Result: PASS — no violations, Complexity Tracking table not required.**

## Documentation Impact

*GATE: Record which user-facing documentation surfaces this feature touches.*

- **Surfaces touched**:
  - `README.md` §Commands table — the `overview` row's one-line description gains "flags repos failing on unsupported/deprecated Copilot models."
  - `docs/src/content/docs/overview.md` — new short section documenting the model-support annotation, the `GH_AW_DEFAULT_MODEL_COPILOT` remediation hint, and the additive `model_support` JSON field; the JSON Output section notes the additive field explicitly.
  - `AGENTS.md` / `CLAUDE.md` — the "Overview dashboard" architecture paragraph gains one sentence on the failure-issue-marker model-support signal and the effective-model resolution seam (this is a new architectural surface: a new `gh` seam + a version-coupled baked-default constant).
- **Update planned**: **yes** — `overview` gains user-visible output (a detail-block line, a stderr hint, an additive JSON field), so the surfaces above are updated in the same change per the constitution's Development Workflow rule.
- **Hidden surfaces**: none. (The `__schema` command is untouched.)

## Project Structure

### Documentation (this feature)

```text
specs/020-overview-model-health/
├── plan.md              # This file (/speckit-plan command output)
├── research.md          # Phase 0 output — data-source + model-attribution decisions
├── data-model.md        # Phase 1 output — ModelSupport entity + RepoOverview field
├── quickstart.md        # Phase 1 output — run + verify recipe
├── contracts/           # Phase 1 output — CLI output contract + upstream signal contracts
│   ├── overview-model-support.md
│   └── upstream-signals.md
├── checklists/
│   └── requirements.md  # Pre-existing spec-quality checklist (all items pass)
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

```text
internal/fleet/
├── overview.go              # + ModelSupport type, RepoOverview.ModelSupport field,
│                            #   collectOverviewModelHealth collector, third wg.Go in Overview,
│                            #   joinOverviewRow wiring  (extract to overview_model.go if >300 lines)
├── overview_test.go         # + model-health reducer/collector tests, stub wiring
├── consumption_logs.go      # unchanged (logsRun stays as-is; detection does NOT extend the logs parse)
├── constants.go             # + fieldWorkflow / fieldModel constants if needed for Diagnostic.Fields
├── diagnostics.go           # remediation reuses DiagHint (no new fleetdiag code required)
├── model_signal.go          # NEW — ghFailureIssuesAPI + ghRepoVariablesAPI seams, marker parser,
│                            #   effective-model resolver, ghAwBakedDefaultCopilotModel constant
├── model_signal_test.go     # NEW — marker-parse + effective-model unit tests (table-driven)
└── testdata/failures/       # NEW — realistic failure-issue body fixtures (model + non-model)

cmd/
├── overview.go              # + render model-support detail line, emit remediation hint,
│                            #   undetermined-only-in-existing-detail-block gating
└── overview_test.go         # + text-render + JSON-envelope assertions for model_support

README.md                                     # Commands-table overview row
docs/src/content/docs/overview.md             # model-support section + JSON note
AGENTS.md / CLAUDE.md                          # Overview-dashboard architecture sentence
```

**Structure Decision**: Single Go CLI module, existing layout. Detection logic that is genuinely new and self-contained (the two `gh` seams, the marker parser, the effective-model resolver, and the baked-default constant) lands in a new `internal/fleet/model_signal.go` so `overview.go` stays focused on the join/reduce responsibility and does not drift past the 300-line refactor threshold. The result type and collector wiring stay in `overview.go` next to the sibling drift/health collectors they run alongside. `cmd/overview.go` gains render-only additions.

## Complexity Tracking

> No Constitution Check violations — table intentionally empty.
