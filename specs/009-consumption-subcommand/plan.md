# Implementation Plan: Consumption Subcommand

**Branch**: `009-consumption-subcommand` | **Date**: 2026-05-13 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/009-consumption-subcommand/spec.md`

## Summary

Add a new read-only subcommand `gh-aw-fleet consumption` that aggregates per-repo `api-consumption-report` outputs across the fleet. Two-layer fetch: discovery via `gh api repos/{owner}/{repo}/discussions` filtered on the `audits` category and the stable `<!-- gh-aw-tracker-id: api-consumption-report-daily -->` HTML-comment marker; data via `gh api repos/{owner}/{repo}/actions/runs/{run_id}/...` reading `aw_info.json` + `run_summary.json` artifacts. Three mutually-exclusive temporal modes (`--latest` default, `--trailing <Nd>`, `--since <YYYY-MM-DD>`) gate which reports are included. One grouping selector `--by repo|profile|cost-center|workflow` pivots the rollup.

The command mirrors `cmd/list.go`'s read-only pattern: stderr breadcrumb (`(loaded fleet.json + fleet.local.json)`), tabwriter text mode, JSON envelope under `--output json` (envelope schema version unchanged — additive). Diagnostics (missing reports, retention-expired artifacts, in-progress reports, upstream format drift) route through the existing `fleet.Diagnostic` type and the envelope's `warnings[]` slot.

No new third-party dependencies. The existing `Profile.Tier` and `RepoSpec.CostCenter` fields (shipped in 007-billing-metadata-fields) are read but not modified. The existing billing-quota hint in `internal/fleet/diagnostics.go:64` already forward-references this subcommand by name — verifying the copy is the FR-025 deliverable.

## Technical Context

**Language/Version**: Go 1.25.8 (per `go.mod`).
**Primary Dependencies**: `github.com/spf13/cobra` v1.10.2 (CLI), `github.com/rs/zerolog` v1.35.1 (stderr structured logging), `encoding/json` (stdlib), `regexp` (stdlib), `os/exec` via the existing `internal/fleet/execlog.go` shell-out wrapper. **No new third-party dependencies introduced by this slice** — within the approved set under Constitution v1.1.0 § Third-Party Dependencies.
**Storage**: N/A. Pure read calls against `gh api` discussions + `gh api` runs artifacts. No on-disk state, no cache, no persisted index (Constitution Principle IV's caching obligation applies to network-derived catalog state like `templates.json`; consumption discovery is per-invocation by FR-022). Output is transient to stdout.
**Testing**: `go test ./internal/fleet/...` and `go test ./cmd/...` with the existing `*_test.go` pattern; `make ci` (`fmt-check vet lint test`) is the local gate. All `gh api` calls go through a package-level injection seam (mirroring `internal/fleet/fetch.go:183`'s `ghAPIJSON` pattern) so unit tests substitute fixture bytes — no live network in CI per SC-007.
**Target Platform**: Linux/macOS/Windows CLI binary; `go build ./...` clean across platforms.
**Project Type**: Single-binary Go CLI (`cmd/` for cobra wiring, `internal/fleet/` for business logic).
**Performance Goals**: Read/aggregation operation; discovery + artifact fetch run serially per repo, well within Constitution Principle IV's 5-minute ceiling for a typical fleet (≤10 repos, ≤7 days trailing window = ~70 artifact fetches worst case). Per-repo wall-clock is bounded by `gh api` round-trip latency. Parallelism is explicit deferred to a follow-up (Assumptions §7).
**Constraints**: Constitution invariants (no dependency churn, no caching, read-only — FR-002). The `cmd.SchemaVersion = 1` envelope contract MUST NOT bump (SC-006). The existing `billingQuotaHint` copy in `internal/fleet/diagnostics.go:64` already forward-references this subcommand; this slice's diagnostic work is to validate the existing copy still reads correctly post-ship and to add new repo-level warnings for missing reports / retention expiry / parse drift.
**Scale/Scope**: Two new files in `internal/fleet/` (`consumption.go`, `consumption_test.go`), one new file in `cmd/` (`consumption.go`), one-line edit to `cmd/root.go` (`AddCommand`), one-line check in `cmd/output.go` (ensure `consumption` is NOT in `rejectJSONMode`'s deny list — it should support JSON envelope), and one new contract doc (`contracts/consumption-output.json`). No edits to `internal/fleet/schema.go` (tier and cost_center already exist). Test fixtures under `internal/fleet/testdata/consumption/`: representative discussion-body payloads, `aw_info.json` with and without the `cost` field, `run_summary.json`. Docs: `CLAUDE.md`/`AGENTS.md` gain a "Common commands" entry; `skills/` may gain a new `fleet-budget-review` skill (deferred to tasks.md scope).

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Per Constitution v1.1.0:

- **I. Thin-Orchestrator Code Quality**. PASS. The command's entire external interaction wraps `gh api` via `os/exec` — consistent with the orchestrator identity. No re-implementation of GitHub API client logic; `gh` does pagination, auth, retries. Discovery and artifact fetch each shell out exactly the way `internal/fleet/fetch.go:184` already does. Exported identifiers will gain full godoc comments per CLAUDE.md "Code self-documentation" rules. `go build` / `go vet` clean is the bar.
- **II. Testing Standards**. PASS. Pure read command — no `--apply`, no dry-run obligation under Principle II.2. Unit tests cover (a) the in-progress × expired × temporal-mode matrix on the policy filter, (b) discussion body parsing via fixture payloads, (c) artifact JSON parsing with `cost`-present and `cost`-absent fixtures, (d) aggregation across multi-profile repos, (e) JSON envelope shape (mirroring `list_result_test.go`). The injection seam for `gh api` means every test runs offline. `make ci` is the gate.
- **III. UX Consistency (Three-Turn Mutation Pattern)**. PASS BY EXEMPTION. The three-turn pattern governs mutating commands. `consumption` is read-only (FR-002). The pattern does not apply, the same way it does not apply to `list` or `status`.
- **IV. Performance**. PASS. Discovery + artifact fetch are serial per repo and serial across repos for v1, accepting the latency hit. Worst-case fleet of 10 repos × 7 days of artifacts = ~70 sequential `gh api` calls, well under the 5-minute ceiling at typical sub-second latency. **No caching** of discovery or artifact results — explicit FR-022. (This is the documented exception class: caching obligation in Principle IV targets *catalog* state like `templates.json` that operators re-read; per-invocation operational state like discussion lists must not be cached because staleness is the failure mode.) Future parallelism is deferred (Assumptions §7); the design does not preclude it (the seam is `for repo := range repos { ... fetch ... }`).
- **Declarative Reconcile Invariants**. PASS. The subcommand reads `fleet.json` / `fleet.local.json` via the existing `LoadConfig` — no schema additions, no writes. `Profile.Tier` and `RepoSpec.CostCenter` are read fields only (introduced by 007-billing-metadata-fields). gpg signing and `git add`/`commit`/`push` restrictions don't apply to a read command. No source-pin changes.
- **Third-Party Dependencies**. PASS. **No additions to `go.mod`'s `require()` block.** `regexp` (stdlib), `encoding/json` (stdlib), `time` (stdlib), `os/exec` (stdlib via existing `execlog.go`). Re-using existing approved direct deps.
- **Development Workflow**. PASS. PR will include `make ci` evidence and a fixture-driven `go run . consumption --output json` sample. `CLAUDE.md`/`AGENTS.md` gain a one-paragraph description of the subcommand and its temporal/grouping flags per FR-027.

**No violations to justify.** Complexity Tracking section below stays empty.

## Project Structure

### Documentation (this feature)

```text
specs/009-consumption-subcommand/
├── plan.md              # This file (/speckit-plan command output)
├── research.md          # Phase 0 output (/speckit-plan command)
├── data-model.md        # Phase 1 output (/speckit-plan command)
├── quickstart.md        # Phase 1 output (/speckit-plan command)
├── contracts/
│   ├── consumption-output.json       # Phase 1 output: JSON envelope shape
│   ├── consumption-text-output.md    # Phase 1 output: tabwriter text shape
│   ├── discussion-discovery.md       # Phase 1 output: gh api discussion query + body-marker parsing contract
│   └── run-artifact-payload.md       # Phase 1 output: aw_info.json + run_summary.json field map
└── tasks.md             # Phase 2 output (/speckit-tasks command - NOT created here)
```

### Source Code (repository root)

Single-binary Go CLI; no separate `tests/` tree (`*_test.go` lives next to the code under test). Files this feature touches:

```text
internal/
├── fleet/
│   ├── consumption.go            # NEW: types (ConsumptionReport, ConsumptionResult, ConsumptionGroup,
│   │                             #      WorkflowConsumption, FetchMode, reportRef) + helpers
│   │                             #      (discoverReports, shouldIncludeReport, fetchRunArtifacts,
│   │                             #      AggregateConsumption). Includes the package-level
│   │                             #      ghDiscussionsAPI / ghRunArtifactAPI injection seams.
│   ├── consumption_test.go       # NEW: table-driven tests for parser + filter + aggregator;
│   │                             #      envelope-shape assertions mirroring list_result_test.go
│   ├── testdata/
│   │   └── consumption/          # NEW fixture tree
│   │       ├── discussion_valid.json
│   │       ├── discussion_in_progress.json
│   │       ├── discussion_expired.json
│   │       ├── aw_info_cost_present.json
│   │       ├── aw_info_cost_absent.json
│   │       └── run_summary.json
│   └── diagnostics.go            # READ-ONLY VERIFY: confirm billingQuotaHint copy still reads
│                                 # correctly (FR-025). No structural change expected.
cmd/
├── consumption.go                # NEW: cobra subcommand; mirrors list.go's read-only pattern
│                                 # (stderr breadcrumb, json envelope or tabwriter)
├── root.go                       # ONE-LINE EDIT: NewRootCmd().AddCommand(newConsumptionCmd(flagDir))
└── output.go                     # VERIFY: `consumption` is NOT in rejectJSONMode's deny list
                                  # (rejectJSONMode is whitelist-shaped — list/deploy/sync/status/upgrade
                                  # are in the JSON-supporting set by default; consumption joins them).

# Documentation (FR-027):
AGENTS.md                         # add a "consumption" paragraph under the architectural framing,
                                  # describing the two-layer fetch + the four group-by axes
                                  # + additive multi-profile semantic + cost-field nil-until-populated
CLAUDE.md                         # already references AGENTS.md; no direct edit needed unless
                                  # the "Common commands" block gains `go run . consumption` (low cost; add)
skills/                           # OPTIONAL: new fleet-budget-review skill if scope allows
                                  # (defer to tasks.md if not strictly required)
```

**Structure Decision**: Single-binary Go CLI, layout already established by the project (`cmd/` + `internal/fleet/`). No new packages. The new `consumption.go` file in `internal/fleet/` keeps the file count and package shape consistent with peer features (`add.go`, `status.go`, `list_result.go`); aggregation logic, parsing helpers, and the injection seams all live in one focused file under the 300-line guidance (Constitution Principle I).

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

*No violations. Section intentionally empty.*
