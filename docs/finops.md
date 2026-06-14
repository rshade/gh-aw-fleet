# FinOps: Cost tracking for agentic workflows

This document frames the two layers of cost tracking available to fleet
operators: the deployed aggregate rollup (Layer 1) and the optional
per-run attribution layer (Layer 2), along with the trade-offs that shape
the fleet's current position.

## Layer 1: Aggregate & inform (deployed)

**Status:** Deployed. Read-only. No enforcement.

The fleet tracks AI-credit usage at the **fleet level** via
`api-consumption-report` (a daily workflow in the `observability-plus`
profile) and rolls it up with `gh-aw-fleet consumption`.

### How it works

1. **Opt a repo in** by adding `observability-plus` to its profiles list
   in `fleet.json` or `fleet.local.json`.
2. **Deploy** the `api-consumption-report` workflow to that repo with
   `gh-aw-fleet deploy --apply`.
3. **Roll up** fleet-wide spend with `gh-aw-fleet consumption`, which
   queries each repo's agentic workflows and sums their usage.

### Data source: `gh aw logs`

The rollup runs `gh aw logs --json` per agentic workflow in each repo
(the ones compiled to `.lock.yml`), pulling AI-credit (AIC) metadata from
the workflow's run history. **The `api-consumption-report` workflow need
not be deployed** — `--source logs` (the default) works against run logs
and needs no reporting workflow, so the rollup is decoupled from
deployment.

Alternative: `--source artifacts` (legacy, transitional) reads from
`api-consumption-report` discussions and run artifacts directly, but the
cost field is structurally nil under Copilot AI-Credits — that path
predates the AIC schema.

### Metrics: AI credits (AIC) and USD

- **AI Credits (AIC)** is the primary unit — the cost in Copilot
  AI-Credits under usage-based billing.
- **USD** is a flat derivation: `AIC × $0.01` (one credit is one cent).
  See `aicToUSDRate` in `internal/fleet/consumption_logs.go`.
- Both are **advisory only** — neither is enforced by the tool. Spending
  limits live in [GitHub's spending controls][spending], not in the fleet
  tool.

[spending]: https://docs.github.com/en/billing

### Grouping: four axes

Drill into cost concentration with `--by`:

```bash
gh-aw-fleet consumption --by repo          # per repository (default)
gh-aw-fleet consumption --by profile       # per profile
gh-aw-fleet consumption --by cost-center   # per cost center (custom tag)
gh-aw-fleet consumption --by workflow      # top spenders by workflow
```

Multi-profile repos contribute additively to every profile group — a repo
with both `default` and `security-plus` appears in both profile totals.
Repos without a `cost_center` tag fold into the literal `<unset>` bucket.

### Temporal modes: window selection

- `--latest` (default) — most-recent valid data per repo.
- `--trailing 7d` — all runs in the trailing 7-day window.
- `--since 2026-06-01` — all runs on or after the given date.

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

Under Copilot AI-Credits, the cost reported by `gh-aw-fleet consumption`
is **AIC × $0.01**, not the final invoice amount. The actual billed cost
depends on:

- Copilot license tier (list price may differ for enterprise)
- Promotion codes, committed-spend discounts, or other adjustments
- The true AIC-to-USD rate in your contract

**AIC is the true unit of cost.** Use USD as a rough reference only; for
invoicing, trust your GitHub billing dashboard.

## Layer 2: Per-run attribution & operate (optional, not deployed)

**Status:** Optional. Not deployed. Requires evaluation.

The agentics [`cost-tracker`][cost-tracker] workflow offers per-run cost
attribution and anomaly alerts, but introduces two structural caveats the
fleet currently does not resolve.

[cost-tracker]: https://github.com/githubnext/agentics

### What cost-tracker does

- **Posts per-run cost** into every PR spawned by an agentic workflow,
  showing the AIC burned in that single run.
- **Raises alert issues** when a run exceeds a configurable cost
  threshold.
- **Captures timestamp, token usage, and model selection**, enabling
  per-run spend analysis.

### Why the fleet doesn't deploy it

#### Caveat 1: currency-model mismatch

`cost-tracker` derives USD from per-model token counts in
`token-usage.jsonl` × a **static pricing table**. That is a **list-price
benchmark**, not the actual Copilot AI-Credit rate. Under Copilot
AI-Credits:

- `aw_info.json` carries **no USD field** — only the AIC count.
- The AIC→USD conversion is opaque to the tool; GitHub computes it from
  usage and licensing internally.
- The pricing table `cost-tracker` uses **does not map to Copilot
  billing**. Its USD output is informative only, not contractual.

**Bottom line:** AIC from Layer 1 is the true cost unit. Layer 2's USD is
a list-price approximation — useful for relative alerting, not invoicing.
Surfacing the real per-run credit cost is upstream-dependent, tracked in
[#59][i59] (surface Copilot credit attribution once the upstream
`aw_info.json` cost field stabilizes).

#### Caveat 2: trigger-coupling mismatch

`cost-tracker` fires on the GitHub `workflow_run` event, keyed to
**specific upstream workflow names** the agentics library expects (e.g.
`agent-implement`, `agent-pr-fix`). A fleet may deploy a different set of
workflows under different names, so the trigger won't match without
hand-editing the workflow's `on:` frontmatter.

**The fleet, a thin orchestrator, never edits workflow frontmatter** —
that boundary prevents a whole class of bugs, but it also means Layer 2
adoption is an operator-driven, per-workflow decision rather than
something the fleet can wire up automatically.

### Roadmap: FinOps issue cluster

These open issues track the fleet's cost-visibility build-out (titles
verified against GitHub):

- [#102][i102] — `gh-aw-fleet forecast`: fleet-wide pre-spend cost
  projection
- [#104][i104] — cost-oriented trigger-risk lint over the resolved fleet
- [#106][i106] — cap-hit diagnostic hints (max-ai-credits / max-turns
  exceeded)
- [#107][i107] — tier-driven `GH_AW_DEFAULT_*` guardrail injection at
  compile
- [#113][i113] — `--source logs`: bounded concurrency + no-download fast
  path
- [#119][i119] — paginate Actions workflow discovery for the logs source
- [#129][i129] — read-only over-budget highlighting in the rollup
  (`--budget`)
- [#59][i59] — surface Copilot credit attribution once `aw_info.json`
  stabilizes upstream
- [#105][i105] — OTel export / agentic-ops MCP: out-of-scope decision
  record

[i102]: https://github.com/rshade/gh-aw-fleet/issues/102
[i104]: https://github.com/rshade/gh-aw-fleet/issues/104
[i105]: https://github.com/rshade/gh-aw-fleet/issues/105
[i106]: https://github.com/rshade/gh-aw-fleet/issues/106
[i107]: https://github.com/rshade/gh-aw-fleet/issues/107
[i113]: https://github.com/rshade/gh-aw-fleet/issues/113
[i119]: https://github.com/rshade/gh-aw-fleet/issues/119
[i129]: https://github.com/rshade/gh-aw-fleet/issues/129
[i59]: https://github.com/rshade/gh-aw-fleet/issues/59

## Quick decision tree

**"How much does each repo spend?"**
→ Layer 1: `gh-aw-fleet consumption --by repo`. No deployment needed.

**"Cost by profile to plan my Copilot budget?"**
→ Layer 1: `gh-aw-fleet consumption --by profile --trailing 30d`.

**"Which workflow burned the most credits last week?"**
→ Layer 1: `gh-aw-fleet consumption --trailing 7d --by workflow`.

**"Cost in every PR to catch expensive runs immediately?"**
→ Layer 2 (`cost-tracker`) can do this, but see Caveat 2 — it needs
trigger edits the fleet won't make for you.

**"Split costs across teams using cost centers?"**
→ Layer 1 with `cost_center` tags:
`gh-aw-fleet consumption --by cost-center`.

## See also

- [`gh-aw-fleet consumption` architecture](../AGENTS.md) — see the
  "Consumption rollup" section in AGENTS.md.
- [fleet-budget-review skill](../skills/fleet-budget-review/SKILL.md) —
  the operator workflow for consumption review.
