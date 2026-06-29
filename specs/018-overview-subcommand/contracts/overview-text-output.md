# Contract: `overview` text output (default mode)

Rendered with `text/tabwriter` (mirrors `printStatus` / `renderConsumptionText`). Written
to **stdout**; the `(loaded fleet.json + fleet.local.json)` breadcrumb and all diagnostics
go to **stderr**.

## Header line (stderr breadcrumb + window)

```
(loaded fleet.json + fleet.local.json)
overview · window: trailing-7d
```

## Main table

Columns, in order: `REPO  DRIFT  RUNS  FAIL  NOOP  HEALTH  AIC  COST`, then a separator and
a `TOTAL` row.

```
REPO              DRIFT     RUNS  FAIL  NOOP  HEALTH     AIC   COST
rshade/finfocus   aligned     42     2    36     95%  118.40  $1.18
acme/widgets      drifted     18     0     4    100%   47.10  $0.47
acme/api          aligned      9     9     0      0%       -      -
acme/idle         aligned      0     0     0      -        -      -
acme/ghost        errored      -     -     -      -        -      -
------------------------------------------------------------------------
TOTAL                         69    11    40     84%  165.50  $1.65
```

### Cell rendering rules

| Condition | RUNS | FAIL | NOOP | HEALTH | AIC | COST |
|---|---|---|---|---|---|---|
| Normal repo | int | int | int | `N%` (rounded) | `0.00` | `$0.00` |
| Only-failed repo (FR-010) | int | =RUNS | 0 | `0%` | `-` | `-` |
| Zero health-counting runs (FR-011) | `0` | `0` | `0` | `-` | `-` | `-` |
| Run-log fan-out failed (`runs_available=false`) | `-` | `-` | `-` | `-` | `-` | `-` |
| Drift query failed | DRIFT=`errored`; run cells render per their own availability | | | | | |

- `-` is the single blank placeholder for "no data / undefined" (nil-until-positive +
  undefined-health). It is **distinct from `0`** (which means a real measured zero).
- `HEALTH` is `round(health_rate * 100)%`. `TOTAL` HEALTH is the **pooled** rate, not an
  average of the per-repo percentages.
- AIC right-aligned 2dp; COST `$`-prefixed 2dp via `aicToUSD`.

## Per-repo detail blocks

After the table, one block per repo that is `drifted`, `errored`, or unhealthy
(`HEALTH < 100%` with `Runs > 0`). Mirrors `printRepoDetail`.

```
acme/widgets (drifted):
  drifted:
    ci-doctor: desired v0.79.2, actual v0.78.0
  runs: 18 (0 failed, 4 no-op) · health 100% · $0.47

acme/api (unhealthy):
  runs: 9 (9 failed, 0 no-op) · health 0% · cost -
  → all runs failed in this window; cost is blank because failed runs report no credits.

acme/ghost (errored):
  drift: could not query repo — repo_inaccessible
  runs: unavailable — repo_inaccessible
```

A repo that is `aligned` AND `HEALTH == 100%` (or has zero runs) gets **no** detail block —
the table row is sufficient.

## Exit code

- `0` when every in-scope repo is `aligned` (regardless of FAIL/NOOP).
- `1` when any in-scope repo is `drifted` or `errored` (FR-018). Run failures never change
  the exit code (FR-019).

## Empty fleet

Zero in-scope repos → exit `0`, an empty table, and a stderr `empty_fleet` diagnostic
(`No repos in scope.`).

## `--latest` NOOP note

In `--latest` mode the NOOP column is best-effort (aggregate-vs-kept-run window mismatch,
research.md §2); a `hint` diagnostic notes this on stderr / in the envelope `hints[]`.
