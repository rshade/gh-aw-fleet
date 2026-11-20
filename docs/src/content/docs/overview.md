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

## Model-Support Advisory

`overview` flags repos whose recent agentic runs failed because the effective
Copilot model is unsupported or deprecated. It reads each repo's open
`agentic-workflows` failure issues and matches gh-aw's machine-readable
`model_not_supported` marker — no per-run logs required. For a flagged repo it
resolves the effective Copilot model (`GH_AW_MODEL_AGENT_COPILOT`, then
`GH_AW_DEFAULT_MODEL_COPILOT`, then gh-aw's baked default) and names it as the
offending model. Both repository variables and organization variables shared
with the repo count; a repository value overrides an organization one.

The advisory surfaces in three places, all additive:

- **Detail block (stdout):** an affected repo's block gains a line, printed
  after the runs line:

  ```text
  rshade/finfocus (unhealthy):
    runs: 4 (4 failed, 0 no-op) · health 0% · cost -
    model-support: FAILING — offending model(s): claude-sonnet-4.6
  ```

  When the model cannot be named (its variables could not be read) the line
  reads `offending model(s): unknown`. A repo whose failure-issue read itself
  failed shows `model-support: undetermined — model-failure signal unavailable`,
  but only when a detail block is already open for another reason — an
  otherwise-clean repo never gains a block.

- **Remediation hint (stderr):** each flagged repo emits exactly one hint naming
  the fix, which takes effect without recompiling the deployed workflows:

  ```text
  [hint] rshade/finfocus: agentic runs are failing on an unsupported/deprecated
         Copilot model. Set the GH_AW_DEFAULT_MODEL_COPILOT Actions variable on
         the repo to a model your Copilot subscription supports (no recompile
         required).
  ```

  When `GH_AW_MODEL_AGENT_COPILOT` supplies the failing model, the hint names
  that variable instead, because it takes precedence over
  `GH_AW_DEFAULT_MODEL_COPILOT` and setting the default alone changes nothing.

- **JSON (`--output json`):** an additive `model_support` field (see below).

The advisory never changes the exit code — a model failure on an otherwise
drift-aligned fleet still exits `0`. A fully healthy fleet prints no model line,
no hint, and no JSON field.

## JSON Output

Use the standard envelope for automation:

```bash
gh-aw-fleet overview --output json
```

The payload contains `result.repos[]`, `result.total`, and diagnostics routed
through envelope `warnings[]` and `hints[]`. The envelope schema version remains
`1`; `overview` is an additive payload.

Each affected or undetermined repo row carries an additive `model_support`
object; clean rows omit it entirely, so existing consumers are byte-unaffected:

```json
{
  "repo": "rshade/finfocus",
  "drift_state": "aligned",
  "model_support": {
    "status": "affected",
    "offending_models": ["claude-sonnet-4.6"]
  }
}
```

`status` is `affected` or `undetermined`; `offending_models` is a sorted,
deduplicated set that is omitted when the model could not be named.

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
