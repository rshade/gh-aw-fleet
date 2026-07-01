---
title: Fleet Overview
description: One-command drift, run-health, no-op, and AI-credit dashboard for the fleet.
---

`gh-aw-fleet overview` is a read-only dashboard that joins three signals into
one row per repository: workflow drift, recent run health, and AI-credit spend.
It ends with a pooled `TOTAL` row and prints detail blocks for repos that need
attention.

```bash
gh-aw-fleet overview [repo...] [flags]
```

## Columns

- `REPO`: repository name from the scoped fleet config.
- `DRIFT`: `aligned`, `drifted`, or `errored`, using the same drift logic as
  `status`.
- `RUNS`: health-counting runs, successes plus failures. Cancelled, skipped,
  and in-progress runs are excluded.
- `FAIL`: failed health-counting runs: `failure`, `timed_out`, and
  `startup_failure`.
- `NOOP`: successful runs that took no action but still consumed credits.
- `HEALTH`: `successes / RUNS`, rendered as a percent. `-` means no
  health-counting runs.
- `AIC`: AI credits summed from successful in-window runs.
- `COST`: derived USD, `AIC * 0.01`.

Blank cells render as `-`, not `0`. A failed-only repo shows `RUNS` and `FAIL`
but leaves `AIC` and `COST` blank because failed runs do not report AI credits.

## Window Selection

The default window is the trailing 7 days because health is a recent operational
signal.

```bash
gh-aw-fleet overview                 # trailing 7 days
gh-aw-fleet overview --trailing 30d
gh-aw-fleet overview --since 2026-06-01
gh-aw-fleet overview --latest
```

`--latest`, `--trailing`, and `--since` are mutually exclusive. `--latest` keeps
the newest run per workflow; in that mode the `NOOP` column is best-effort
because `gh aw logs` reports no-op usage as an aggregate over the fetched runs,
not as a per-run flag.

## Scoping

Pass one or more tracked repos to scope the dashboard and the `TOTAL` row:

```bash
gh-aw-fleet overview rshade/finfocus hvmesh/hvmesh
```

Unknown repos fail before any GitHub or `gh aw logs` fetch.

## Exit Contract

`overview` is CI-safe as a drift gate:

- Exit `0` when every in-scope repo is `aligned`.
- Exit `1` when any in-scope repo is `drifted` or `errored`.

Run failures are advisory. A fully aligned fleet with failing agentic runs still
exits `0`.

## JSON Output

Use the standard envelope for automation:

```bash
gh-aw-fleet overview --output json
```

The payload contains `result.repos[]`, `result.total`, and diagnostics routed
through envelope `warnings[]` and `hints[]`. The envelope schema version remains
`1`; `overview` is an additive payload.

## Performance

The command reuses the same logs fan-out as `consumption`: list compiled
agentic workflows via the Actions API, then run `gh aw logs --json` per workflow
using the GitHub Actions display name. That path downloads run artifacts and can
take minutes on larger fleets. Drift and run-health batches run concurrently,
but the logs fan-out is still the slow path.

## See Also

- [`gh-aw-fleet consumption`](/gh-aw-fleet/consumption/) for grouped AIC rollups by repo,
  profile, cost center, or workflow.
- [`AGENTS.md`](https://github.com/rshade/gh-aw-fleet/blob/main/AGENTS.md) for
  implementation notes on the shared `gh aw logs` fan-out.
