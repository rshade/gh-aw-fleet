# FinOps cost model

Agentic workflows on this fleet bill in GitHub Copilot AI credits (AIC).
Cost visibility is two layers. Only the first one is deployed.

## Layer 1 — aggregate rollup

`observability-plus` can install `api-consumption-report` (daily, from
`github/gh-aw`). The operator-facing rollup is `gh-aw-fleet consumption`.

The default `--source logs` does not need that workflow. For each repo it
lists agentic workflows (compiled `.lock.yml` files) with the Actions API,
then runs `gh aw logs --json` per workflow display name and sums
`summary.total_aic`. USD in the `COST` column is `AIC * 0.01`
(`aicToUSDRate` in `internal/fleet/consumption_logs.go`). Group with
`--by repo|profile|cost-center|workflow`.

This path is pull-based. It does not set caps, open issues, or change the
exit code when spend is high. `--budget` only marks rows whose AIC is above
a ceiling. Spending limits stay in
[GitHub's spending controls](https://github.com/settings/billing/spending_limit).

`--source artifacts` is the transitional fallback. It discovers
`api-consumption-report` posts in the `audits` discussion category and reads
`aw_info.json` plus `run_summary.json`. Under Copilot billing that artifact
has no USD field, so its `COST` column is usually empty.

## Layer 2 — per-run attribution (not deployed)

The agentics `cost-tracker` workflow posts a per-run cost on the PR and can
open a high-spend issue over a threshold. The fleet does not install it.

Two reasons:

- **Currency.** `cost-tracker` multiplies `token-usage.jsonl` by a per-model
  price table and reports USD. This fleet is invoiced in Copilot AIC.
  `aw_info.json` carries no USD under Copilot, so that USD figure is a
  list-price benchmark, not the bill. AIC stays the unit that matches the
  invoice. See
  [#59](https://github.com/rshade/gh-aw-fleet/issues/59).
- **Triggers.** `cost-tracker` runs on `workflow_run` for upstream workflow
  names such as `agent-implement` and `agent-pr-fix`. Those names are not
  the fleet's deployed set (`ci-doctor`, `pr-fix`, and the rest of
  `profiles/default.json`). It will not fire unless that frontmatter
  changes. The fleet does not rewrite workflow markdown.

## Roadmap

Later FinOps work, not part of this map:

- [#102](https://github.com/rshade/gh-aw-fleet/issues/102) forecast
- [#104](https://github.com/rshade/gh-aw-fleet/issues/104) trigger-risk lint
- [#105](https://github.com/rshade/gh-aw-fleet/issues/105) OTel / agentic-ops decision
- [#106](https://github.com/rshade/gh-aw-fleet/issues/106) cap-hit hints
- [#107](https://github.com/rshade/gh-aw-fleet/issues/107) tier guardrails at compile
- [#113](https://github.com/rshade/gh-aw-fleet/issues/113) consumption follow-up
- [#119](https://github.com/rshade/gh-aw-fleet/issues/119) paginate workflow discovery
- [#59](https://github.com/rshade/gh-aw-fleet/issues/59) Copilot credit attribution
