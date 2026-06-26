# Phase 0 Research: Fleet Overview Subcommand

All "NEEDS CLARIFICATION" from Technical Context resolved below. Several decisions
were grounded in a **live investigation of the real fleet** (`rshade/finfocus`,
`hvmesh/hvmesh`) during specification — those are flagged ⬡.

## 1. Health taxonomy — which conclusions count as failures

**Decision**: Failures-only. `success` → healthy; `failure` / `timed_out` /
`startup_failure` → failure; `cancelled` / `skipped` / `""` (non-terminal) →
**excluded from the denominator entirely**. `RUNS = successes + failures`;
`HEALTH = successes ÷ RUNS`; `RUNS` is the displayed denominator so the arithmetic
is self-evident. A single `classifyConclusion(string) (healthy, failed, counted bool)`
helper encodes it.

**Rationale**: ⬡ Confirmed against the live fleet that agentic workflows are
cancelled constantly by concurrency rules on rapid pushes; folding `cancelled` into
FAIL would make a healthy fleet read as broken. Resolved with the operator during
specification (FR-007). Matches the issue's own sample (`9 runs / 9 fail / 0%` — all
genuine `failure` conclusions).

**Alternatives considered**: (a) "anything-not-success is a failure" — rejected: a
push-storm of cancellations reads as a failing fleet. (b) Exclude no-ops from the
denominator too (productive-rate health) — rejected: contradicts upstream's "no-ops
ARE successes" framing and shrinks the denominator to noise.

## 2. No-op detection — source field and per-repo derivation ⬡

**Decision**: Derive `NOOP` from the **aggregate** `mcp_tool_usage.summary[]` entry
where `server_name == "safeoutputs"` and `tool_name == "noop"`, taking its
`call_count`, summed per repo across the workflow `gh aw logs` calls, then **clamped
to `[0, successes]`**. This requires extending `logsPayload` with an `mcp_tool_usage`
field (currently dropped on unmarshal — additive, breaks nothing).

**Rationale**: ⬡ Empirically verified by running the real `gh aw logs --json --repo
rshade/finfocus --start-date -30d -c 1000 "Sub-Issue Closer"`:

- The per-run object exposes **no** no-op signal in gh-aw v0.79.2. `safe_items_count`
  is absent/`0` even when safe outputs occurred; `classification` is `normal`/`baseline`
  (unrelated). So per-run no-op inference is impossible.
- `mcp_tool_usage.summary` carried `{server_name: safeoutputs, tool_name: noop,
  call_count: 9}`, and the call had exactly **9 successful runs** (20 failures, 9
  successes) — a clean 1:1 (one `noop` safe-output per no-op run).
- A single no-op run cost **aic ≈ 313 (~$3)** — confirms the column's value: no-ops
  are real recurring spend for zero output.

**Windowing caveat**: `mcp_tool_usage` is an aggregate over the `gh aw logs` call's
window (`--start-date`), not per-run, so it is **not** client-side re-filterable the
way `runs[]` are. For `--trailing`/`--since` the `--start-date` equals the reporting
window, so they align. For `--latest` (`-c 5`, no `--start-date`) the aggregate spans
up to 5 runs while `filterRunsByWindow` keeps 1 — a mismatch. Mitigation: clamp
`NOOP ≤ successes-in-window`; document `--latest` NOOP as best-effort in `--help` and
`docs/overview.md`. The exact-per-window fallback (the `[aw] No-Op Runs` issue feed,
marker `<!-- gh-aw-noop-runs -->`) is **not implemented this slice** — the aggregate +
clamp is sufficient for the default trailing window and avoids a third API path.

**Alternatives considered**: (a) infer no-op from `safe_items_count == 0` on a success
— rejected: the field is unpopulated in v0.79.2 (verified). (b) read the `[aw] No-Op
Runs` issue feed — rejected for this slice: adds a third per-repo API path for marginal
precision; kept as documented future fallback. (c) defer the column — rejected: the
user explicitly prioritized making run-rate waste visible.

## 3. Factoring the shared per-repo fan-out

**Decision**: Extract a behavior-preserving helper from `logSourceToReports` —
`collectRepoRuns(ctx, repo, mode, now) ([]repoRunData, []Diagnostic)` (the `repoRunData`
struct is defined in data-model.md) — that runs `ghWorkflowsAPI` → `ghLogsAPI` →
`filterRunsByWindow` once per repo and yields, per workflow, the filtered `[]logsRun`
**plus** the decoded `mcp_tool_usage`.
`logSourceToReports` refactors to call it (then summarize into `ConsumptionReport` as
today); `overview.go`'s reducer calls it to compute `{Runs, Failures, NoOps, AIC, Cost}`.

**Rationale**: Satisfies FR-004 ("MUST NOT re-implement that fan-out") literally — one
fan-out, two consumers — and the spec's "factor the fan-out so both callers share it."
Keeps the documented `gh aw logs` quirks (display-name-vs-slug, throwaway temp dir,
version gate) in exactly one place. The existing `consumption_test.go` suite guards the
refactor against regression.

**Alternatives considered**: (a) a sibling extractor that independently calls the same
seams — rejected: duplicates the per-workflow loop and the window filter, drifting from
`consumption` over time. (b) leave `logSourceToReports` untouched and have overview call
`AggregateConsumption` then reach into groups — rejected: `AggregateConsumption` returns
*grouped* rows and discards per-repo run/failure/noop counts; overview needs the raw
per-repo health, which only the inner fan-out has.

## 4. The drift × health × cost join + per-repo isolation

**Decision**: `Overview` drives the row set from the **scoped `cfg.Repos`** (after
`ScopeToRepos`), then populates each row independently: drift from a `map[repo]RepoStatus`
built from `Status()`'s result, health/cost from a `map[repo]healthCost` built from the
fan-out. Each side is independently nil-able — a repo errored in `Status()` still gets a
row (drift cell = errored, health from the fan-out); a repo whose fan-out failed still
shows drift. Per-repo fetch errors become `fleet.Diagnostic`s (FR-020).

**Rationale**: Mirrors how `status` already isolates per-repo errors. Building the row
set from the declared fleet (not from either result) guarantees no repo silently
vanishes and that the two half-results join cleanly even when one is missing.

**Alternatives considered**: drive rows from the union of the two result sets — rejected:
a repo absent from both (e.g. no workflows + drift-skipped) would disappear; the declared
fleet is the authoritative row set.

## 5. Concurrency model

**Decision**: Run the drift batch (`Status()` for all scoped repos) and the health/cost
batch (the fan-out for all scoped repos) **concurrently in two goroutines**, join after
both complete. Within each batch, keep the existing per-repo behavior (`Status` already
parallelizes internally; the logs fan-out stays serial-per-repo as in `consumption`).

**Rationale**: Constitution IV ("SHOULD parallelize I/O-bound independent targets") — the
two batches are independent and the logs batch is the slow path, so overlapping them with
drift is a near-free win. Bounded per-repo concurrency inside the logs fan-out (#113) is
an upstream improvement to **consume when it lands**, not re-solve here (Assumptions).

**Alternatives considered**: fully serial (drift then health) — rejected: needlessly
doubles wall-clock. Per-repo goroutine fan-out across both signals now — rejected:
re-solves #113 prematurely and complicates the seam.

## 6. Exit-code disposition

**Decision**: Compute an overall disposition from drift only: non-zero
(`newCommandExitError(errOverviewDrift, 1, true)`) iff the pooled
`OverviewTotal.Drifted + OverviewTotal.Errored > 0` (equivalently, any in-scope repo's
`DriftState != DriftStateAligned`); zero otherwise — independent of run failures
(FR-018/FR-019). Reuse `status`'s exact mechanism (`cmd/exit.go`'s `commandExitError`,
`silent=true`). Reading the `Total` tallies (which the JSON contract already requires) avoids a
second per-repo scan duplicating `statusExitCode`.

**Rationale**: Makes `overview` a drop-in CI drift gate that does not flap on flaky
agentic runs. Identical semantics to `status` so operators already know the contract.

**Alternatives considered**: gate on run failures too — rejected: explicitly out of scope;
the opt-in `--fail-on-runs` is roadmap #155.

## 7. Envelope, command name, and JSON support

**Decision**: Add `commandOverview = "overview"` to `cmd/output.go`; emit via
`writeEnvelope(cmd, commandOverview, "", false, res, warnings, hints)`. `overview` does
**not** call `rejectJSONMode`, so `--output json` is supported by default. `cmd.SchemaVersion`
stays `1`.

**Rationale**: `rejectJSONMode` is a deny-helper invoked only by commands that *don't*
support JSON (`template fetch`, `add`); omitting the call is how a command opts in. The new
`OverviewResult` payload is additive under the existing envelope, so no schema bump (SC-004).

**Alternatives considered**: a per-command JSON allow-list — rejected: not how the codebase
works; opt-in-by-omission is the established pattern (`list`/`status`/`consumption`).

## 8. Diagnostics — reuse existing codes

**Decision**: Reuse `DiagRepoInaccessible`, `DiagRateLimited`, `DiagEmptyFleet`, and the
`gh aw` version-gate diags (`DiagGhAwTooOld`/`DiagGhAwMissing`, inherited from the shared
fan-out). Route a per-repo fan-out failure as `DiagRepoInaccessible` with `Fields["repo"]`
and `Fields["signal"] = "runs"`; a drift error similarly with `signal = "drift"`. Use
`DiagHint` for the `--latest` NOOP best-effort note. **No new diag code** unless a
genuinely new join-time condition emerges (none identified).

**Rationale**: FR-024 mandates reuse; the existing code set covers the surface. `Fields`
carries the per-repo / per-signal context without a new code.

**Alternatives considered**: a new `overview_*` diag family — rejected: nothing here is a
new *class* of error; they are the same inaccessible/rate-limited/empty conditions the
existing codes describe.

## 9. Open items intentionally deferred

- Bounded per-repo concurrency / no-download fast path for the logs fan-out → **#113**
  (consume when available).
- Workflow-discovery pagination beyond `per_page=100` → **#119**.
- `--fail-on-runs` exit-gate on run failures → **#155**.
- `[aw] No-Op Runs` issue-feed exact-window NOOP source → future fallback (not needed for
  the default trailing window).
- Per-workflow / per-profile / per-cost-center grouping of the overview → future slice
  (overview is per-repo first).
- Retention-boundary attribution for `--since` set beyond run-log retention → deferred. A
  truncated span currently renders as a quiet window (fewer/zero health-counting runs);
  emitting a dedicated diagnostic that names retention as the cause would require parsing
  `gh aw logs` output for a retention signal, which no existing diag code covers and which §8
  rules out adding this slice. The clamp/render-what-remains behavior (spec Edge Cases) is the
  intended best-effort result.
