---
title: Consumption and FinOps
description: Cost tracking for agentic workflows with AI-credit rollups and optional per-run attribution.
---

This page frames the two layers of cost tracking available to fleet operators:
the deployed aggregate rollup (Layer 1) and the optional per-run attribution
layer (Layer 2), along with the trade-offs that shape the fleet's current
position.

## Layer 1: Aggregate and inform

**Status:** Deployed. Read-only. No enforcement.

The fleet tracks AI-credit usage at the fleet level via
`api-consumption-report` (a daily workflow in the `observability-plus` profile)
and rolls it up with `gh-aw-fleet consumption`.

### How it works

1. Opt a repo in by adding `observability-plus` to its profiles list in
   `fleet.json` or `fleet.local.json`.
2. Deploy the `api-consumption-report` workflow to that repo with
   `gh-aw-fleet deploy --apply`.
3. Roll up fleet-wide spend with `gh-aw-fleet consumption`, which queries each
   repo's agentic workflows and sums their usage.

### Data source: gh aw logs

The rollup runs `gh aw logs --json` per agentic workflow in each repo (the ones
compiled to `.lock.yml`), pulling AI-credit (AIC) metadata from the workflow's
run history. The `api-consumption-report` workflow need not be deployed:
`--source logs` (the default) works against run logs and needs no reporting
workflow, so the rollup is decoupled from deployment.

Alternative: `--source artifacts` (legacy, transitional) reads from
`api-consumption-report` discussions and run artifacts directly, but the cost
field is structurally nil under Copilot AI-Credits. That path predates the AIC
schema.

### Metrics: AI credits and USD

- **AI Credits (AIC)** is the primary unit - the cost in Copilot AI-Credits
  under usage-based billing.
- **USD** is a flat derivation: `AIC * $0.01` (one credit is one cent). See
  `aicToUSDRate` in `internal/fleet/consumption_logs.go`.
- Both are advisory only. Neither is enforced by the tool. Spending limits live
  in [GitHub's spending controls](https://docs.github.com/en/billing), not in
  the fleet tool.

### Grouping: four axes

Drill into cost concentration with `--by`:

```bash
gh-aw-fleet consumption --by repo          # per repository (default)
gh-aw-fleet consumption --by profile       # per profile
gh-aw-fleet consumption --by cost-center   # per cost center (custom tag)
gh-aw-fleet consumption --by workflow      # top spenders by workflow
```

Multi-profile repos contribute additively to every profile group: a repo with
both `default` and `security-plus` appears in both profile totals. Repos without
a `cost_center` tag fold into the literal `<unset>` bucket.

### Temporal modes: window selection

- `--latest` (default): most-recent valid data per repo.
- `--trailing 7d`: all runs in the trailing 7-day window.
- `--since 2026-06-01`: all runs on or after the given date.

### Budget highlighting: `--budget`

Pass `--budget <AIC>` to flag rows whose AI-credit total strictly exceeds the
given ceiling. It is read-only: it surfaces cost concentration at a glance but
never enforces or blocks anything.

```bash
# Flag any group (and top burner) over 500 AIC
gh-aw-fleet consumption --by repo --budget 500
```

With a ceiling set, the table and the `TOP 10 BURNERS` footer gain a trailing
`OVER` column that marks each over-ceiling row with `!`. In `--output json`, the
same signal appears as an over-budget field on each group instead of a column.
The ceiling must be a finite, non-negative number.

### Command examples

```bash
# Fleet spend grouped by profile, scoped to the last 7 days
gh-aw-fleet consumption --trailing 7d --by profile

# Drill into one repo's per-workflow cost
gh-aw-fleet consumption rshade/my-repo --by workflow

# Export JSON for integration with cost-management systems
gh-aw-fleet consumption --output json
```

### AIC is the metric; USD is approximate

Under Copilot AI-Credits, the cost reported by `gh-aw-fleet consumption` is
`AIC * $0.01`, not the final invoice amount. The actual billed cost depends on:

- Copilot license tier (list price may differ for enterprise).
- Promotion codes, committed-spend discounts, or other adjustments.
- The true AIC-to-USD rate in your contract.

**AIC is the true unit of cost.** Use USD as a rough reference only. For
invoicing, trust your GitHub billing dashboard.

## Layer 2: Per-run attribution and operate

**Status:** Optional. Not deployed. Requires evaluation.

The agentics [`cost-tracker`](https://github.com/githubnext/agentics) workflow
offers per-run cost attribution and anomaly alerts, but introduces two
structural caveats the fleet currently does not resolve.

### What cost-tracker does

- Posts per-run cost into every PR spawned by an agentic workflow, showing the
  AIC burned in that single run.
- Raises alert issues when a run exceeds a configurable cost threshold.
- Captures timestamp, token usage, and model selection, enabling per-run spend
  analysis.

### Why the fleet does not deploy it

#### Caveat 1: currency-model mismatch

`cost-tracker` derives USD from per-model token counts in `token-usage.jsonl`
multiplied by a static pricing table. That is a list-price benchmark, not the
actual Copilot AI-Credit rate. Under Copilot AI-Credits:

- `aw_info.json` carries no USD field: only the AIC count.
- The AIC-to-USD conversion is opaque to the tool. GitHub computes it from usage
  and licensing internally.
- The pricing table `cost-tracker` uses does not map to Copilot billing. Its USD
  output is informative only, not contractual.

Bottom line: AIC from Layer 1 is the true cost unit. Layer 2's USD is a
list-price approximation - useful for relative alerting, not invoicing.
Surfacing the real per-run credit cost is upstream-dependent, tracked in
[#59](https://github.com/rshade/gh-aw-fleet/issues/59).

#### Caveat 2: trigger-coupling mismatch

`cost-tracker` fires on the GitHub `workflow_run` event, keyed to specific
upstream workflow names the agentics library expects, such as `agent-implement`
or `agent-pr-fix`. A fleet may deploy a different set of workflows under
different names, so the trigger will not match without hand-editing the
workflow's `on:` frontmatter.

The fleet, a thin orchestrator, never edits workflow frontmatter. That boundary
prevents a class of bugs, but it also means Layer 2 adoption is an
operator-driven, per-workflow decision rather than something the fleet can wire
up automatically.

### Roadmap: FinOps issue cluster

These open issues track the fleet's cost-visibility build-out:

- [#102](https://github.com/rshade/gh-aw-fleet/issues/102):
  `gh-aw-fleet forecast`, fleet-wide pre-spend cost projection.
- [#104](https://github.com/rshade/gh-aw-fleet/issues/104):
  cost-oriented trigger-risk lint over the resolved fleet.
- [#106](https://github.com/rshade/gh-aw-fleet/issues/106):
  cap-hit diagnostic hints (`max-ai-credits` / `max-turns` exceeded).
- [#107](https://github.com/rshade/gh-aw-fleet/issues/107):
  tier-driven `GH_AW_DEFAULT_*` guardrail injection at compile.
- [#113](https://github.com/rshade/gh-aw-fleet/issues/113):
  `--source logs`, bounded concurrency and no-download fast path.
- [#119](https://github.com/rshade/gh-aw-fleet/issues/119):
  paginate Actions workflow discovery for the logs source.
- [#59](https://github.com/rshade/gh-aw-fleet/issues/59):
  surface Copilot credit attribution once `aw_info.json` stabilizes upstream.
- [#105](https://github.com/rshade/gh-aw-fleet/issues/105):
  OTel export and agentic-ops MCP out-of-scope decision record.

## Quick decision tree

**How much does each repo spend?**  
Use Layer 1: `gh-aw-fleet consumption --by repo`. No deployment needed.

**Cost by profile to plan my Copilot budget?**  
Use Layer 1: `gh-aw-fleet consumption --by profile --trailing 30d`.

**Which workflow burned the most credits last week?**  
Use Layer 1: `gh-aw-fleet consumption --trailing 7d --by workflow`.

**Cost in every PR to catch expensive runs immediately?**  
Layer 2 (`cost-tracker`) can do this, but see the trigger-coupling caveat. It
needs trigger edits the fleet will not make for you.

**Split costs across teams using cost centers?**  
Use Layer 1 with `cost_center` tags:
`gh-aw-fleet consumption --by cost-center`.

## See also

- [`gh-aw-fleet consumption` architecture](https://github.com/rshade/gh-aw-fleet/blob/main/AGENTS.md):
  see the "Consumption rollup" section in `AGENTS.md`.
- [fleet-budget-review skill](https://github.com/rshade/gh-aw-fleet/blob/main/skills/fleet-budget-review/SKILL.md):
  the operator workflow for consumption review.
