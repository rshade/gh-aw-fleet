# Contract: run-health + run-rate extraction from `gh aw logs --json`

How `overview` derives `RUNS / FAIL / NOOP / HEALTH / AIC / COST` per repo from the shared
`gh aw logs` fan-out. This is the reuse boundary for FR-003/FR-004/FR-007/FR-031.

## Input

Per `(repo, workflow)`, the shared `collectRepoRuns` helper returns the window-filtered
`[]logsRun` (via `filterRunsByWindow`) plus the decoded `mcpToolUsage`. A repo aggregates
across all its workflows.

`logsRun` fields consumed: `Conclusion` (string), `AIC` (`*float64`, nil on non-success),
`CreatedAt`. `mcpToolUsage.summary[]` consumed: the `{server_name:"safeoutputs",
tool_name:"noop"}` entry's `call_count`.

## Conclusion → health classification (FR-007, failures-only)

```
classifyConclusion(c):
  "success"                                   -> {healthy:true,  failed:false, counted:true}
  "failure" | "timed_out" | "startup_failure" -> {healthy:false, failed:true,  counted:true}
  "cancelled" | "skipped" | "" (non-terminal) -> {healthy:false, failed:false, counted:false}
  (any other / unknown terminal conclusion)   -> treat as failure (counted:true, failed:true)
```

Unknown terminal conclusions count as failures (fail-safe: a new bad outcome should not be
silently dropped from health). `cancelled`/`skipped`/empty are explicitly excluded.

## Per-repo reduction

```
successes = count(runs where counted && healthy)
failures  = count(runs where counted && failed)
RUNS      = successes + failures              # the displayed health denominator
HEALTH    = successes / RUNS                  # nil when RUNS == 0
AIC       = sum(run.AIC where run.AIC != nil && > 0)   # nil-until-positive
COST      = aicToUSD(AIC)                      # nil when AIC nil/non-positive

noopRaw   = sum over workflows of noopCount(payload)   # safeoutputs/noop call_count
NOOP      = clamp(noopRaw, 0, successes)
```

### Invariants enforced

- `0 ≤ failures ≤ RUNS`; `0 ≤ NOOP ≤ successes = RUNS − failures`.
- `RUNS == 0` ⇒ `HEALTH == nil`, `AIC/COST == nil`, `NOOP == 0`.
- A repo with only failed runs ⇒ `failures == RUNS`, `HEALTH == 0`, `AIC/COST == nil`.

## No-op windowing caveat (FR-031, research.md §2)

`mcp_tool_usage` is an **aggregate over the `gh aw logs` call window** (`--start-date`), not
per-run, so it cannot be re-filtered by `created_at` the way `runs[]` are.

| Mode | `gh aw logs` flags | NOOP accuracy |
|---|---|---|
| `--trailing Nd` | `--start-date -Nd -c 1000` | aggregate window == reporting window → accurate (modulo `-c 1000` cap) |
| `--since DATE` | `--start-date DATE -c 1000` | accurate (modulo cap) |
| `--latest` | `-c 5` (no start-date) | best-effort: aggregate spans up to 5 runs, runs re-filtered to 1 → clamp to successes; emit a `hint` diagnostic |

The clamp `NOOP ≤ successes` bounds the error. Exact per-window counts (if ever needed) would
come from the `[aw] No-Op Runs` issue feed (`<!-- gh-aw-noop-runs -->`) — **not** this slice.

## Empirical validation (live fleet, 2026-06-23)

`gh aw logs --json --repo rshade/finfocus --start-date -30d -c 1000 "Sub-Issue Closer"`:
29 runs → 20 `failure`, 9 `success`; `mcp_tool_usage` `safeoutputs/noop` `call_count == 9`
(exact 1:1 with successes). One no-op success carried `aic ≈ 313` (~$3). Per-run
`safe_items_count` absent; `classification` was `normal`/`baseline` (no per-run no-op signal
in gh-aw v0.79.2). This is the basis for the aggregate-with-clamp approach.

## Version gate (inherited)

`--source logs`-style fan-out requires gh-aw ≥ the existing `logsSourceMinVersion`
(`ensureLogsSourceGhAwVersion`). Overview inherits this gate via the shared fan-out — a too-old
CLI surfaces `DiagGhAwTooOld`/`DiagGhAwMissing` rather than silently empty health columns.

## Per-repo error isolation (FR-020)

A `ghWorkflowsAPI`/`ghLogsAPI` failure for one repo sets `RunsAvailable=false` + `RunsError`
on that row and emits a `repo_inaccessible` (or `rate_limited`) diagnostic with
`Fields{"repo", "signal":"runs"}`; the repo still renders (drift from the other batch), and
every other repo renders normally.
