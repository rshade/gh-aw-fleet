# Quickstart: `gh-aw-fleet overview`

One read-only command that joins **drift**, **run health**, and **cost + run-rate** across
the fleet into a single dashboard. Reuses the drift path (`status`) and the `gh aw logs`
fan-out (`consumption`) — no new API logic, no new dependencies, no persisted state.

## Usage

```bash
gh-aw-fleet overview [repo...] [flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `--trailing <Nd>` | `7d` | Window = trailing N days (the default window). |
| `--since <YYYY-MM-DD>` | — | Window = from a fixed date forward. |
| `--latest` | — | Single most-recent report per workflow. NOOP is best-effort in this mode. |
| `--output text\|json` | `text` | `json` emits the standard envelope. |

`--trailing` / `--since` / `--latest` are **mutually exclusive**. With no temporal flag the
window is the **trailing 7 days** (health is a windowed concept — this differs from
`consumption`'s `--latest` default).

```bash
gh-aw-fleet overview                          # whole fleet, last 7 days
gh-aw-fleet overview rshade/finfocus          # one repo
gh-aw-fleet overview --since 2026-06-01       # since a date
gh-aw-fleet overview --output json | jq .     # machine-readable
```

## Reading the dashboard

```
REPO              DRIFT     RUNS  FAIL  NOOP  HEALTH     AIC   COST
rshade/finfocus   aligned     42     2    36     95%  118.40  $1.18
acme/api          aligned      9     9     0      0%       -      -
------------------------------------------------------------------------
TOTAL                         69    11    40     84%  165.50  $1.65
```

- **DRIFT** — `aligned` / `drifted` / `errored` (desired vs actual workflow refs).
- **RUNS** — health-counting runs = successes + failures. `cancelled`/`skipped`/in-progress
  runs are **excluded** (they don't reflect on health).
- **FAIL** — `failure` / `timed_out` / `startup_failure` runs.
- **NOOP** — successful runs that took no action ("nothing to do"). They're **healthy** but
  still cost credits — this column is the run-rate-waste signal.
- **HEALTH** — `successes ÷ RUNS`. `-` means no health-counting runs (undefined, not 0%).
- **AIC / COST** — AI credits and derived USD (`AIC × 0.01`). `-` (not `0`) when absent;
  failed-only repos show `-` because failed runs report no credits.

A repo that is drifted, errored, or unhealthy gets a detail block below the table.

## Exit code (CI-safe drift gate)

- **`0`** — every in-scope repo is `aligned`, *even if runs failed* (run failures are advisory).
- **`1`** — any repo is `drifted` or `errored`.

So `overview` can gate a CI pipeline on fleet drift without flapping on a flaky agentic run.
(An opt-in `--fail-on-runs` is roadmap #155.)

```bash
gh-aw-fleet overview --output json >/dev/null || echo "fleet has drifted"
```

## Performance note

The `gh aw logs` fan-out is the slow path — it enumerates each repo's agentic workflows and
downloads run artifacts per workflow. Expect minutes on a ~10-repo fleet; the drift and
health/cost batches run concurrently. Bounded concurrency / a no-download fast path (#113)
and discovery pagination (#119) are upstream improvements this command will consume.

## Try it locally (offline, against fixtures)

```bash
go test ./internal/fleet/ -run TestOverview
go test ./cmd/ -run TestOverviewCmd
make ci          # full gate: fmt-check vet lint test
```

All external calls are injected seams (`ghWorkflowsAPI`, `ghLogsAPI`, the `Status` fetcher),
so the suite runs with no network. Live smoke test (needs `gh` auth + `gh aw` ≥ pinned):

```bash
go run . overview --trailing 7d
```
