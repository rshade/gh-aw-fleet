# Quickstart: `gh-aw-fleet consumption`

**Feature**: 009-consumption-subcommand
**Plan**: [plan.md](./plan.md)
**Audience**: Operator using the fleet CLI after this feature ships.

## Prerequisites

1. Fleet config (`fleet.local.json` for real fleet state, `fleet.json` for the public example) declares at least one repo.
2. The upstream `api-consumption-report` workflow has been deployed to at least some of those repos (separate roadmap issue — pair this command's rollout with that deployment).
3. `gh` CLI authenticated against an account/PAT that can read the target repos' Discussions and Actions runs.

## The first useful command

```bash
gh-aw-fleet consumption
```

What it does:

- Loads `fleet.json` + `fleet.local.json` (existing merge precedence).
- For each repo in the config, queries the repo's `audits`-category discussions matching the `api-consumption-report-daily` tracker marker.
- Picks the single most-recent valid (non-in-progress, non-expired) report per repo.
- Fetches that report's structured artifacts (`aw_info.json` + `run_summary.json`) from the workflow run it references.
- Prints a tabwriter table grouped by repo, plus a TOP 10 BURNERS footer of the highest-consuming workflows.

Sample output:

```text
  (loaded fleet.json + fleet.local.json)
REPO                      API_CALLS  SAFE_WRITES  COST     REPORTS
rshade/example-paid       4827       31           $12.45   1
rshade/example-minimal    612        4            -        1
rshade/example-spotty     1100       7            -        1

TOP 10 BURNERS:
WORKFLOW                    RUNS  API_CALLS  AVG_DURATION  COST
issue-triage                23    3210       42.7s         $8.91
ci-doctor                   12    2104       31.2s         -
api-consumption-report      3     805        18.4s         -
```

## Common follow-up commands

### "What did the fleet consume over the last seven days?"

```bash
gh-aw-fleet consumption --trailing 7d
```

Same shape, but each row sums seven days of reports per repo.

### "What did the fleet consume since we onboarded the new team?"

```bash
gh-aw-fleet consumption --since 2026-04-01
```

Same shape, but the temporal window opens at the given calendar date and continues to now.

### "Which profiles are the most expensive?"

```bash
gh-aw-fleet consumption --by profile
```

Pivots the table key column from REPO to PROFILE. A repo participating in two profiles contributes its full consumption to both rows (additive double-counting — see [research.md](./research.md) Decision 5).

### "Where does the spend attribute by cost center?"

```bash
gh-aw-fleet consumption --by cost-center
```

Pivots on the `cost_center` value declared on each `RepoSpec` in fleet config (shipped in 007-billing-metadata-fields). Repos without a `cost_center` land under the `<unset>` bucket — they don't disappear.

### "Which individual workflows are driving the cost?"

```bash
gh-aw-fleet consumption --by workflow
```

Pivots away from per-repo aggregation entirely; each row is one workflow name with consumption summed across every repo that runs it.

### "JSON for my dashboard / jq pipeline"

```bash
gh-aw-fleet consumption --output json --trailing 7d --by cost-center
```

Emits the standard fleet JSON envelope. `result.groups[]` carries the rows; `result.top_burners[]` carries the burner footer; `warnings[]` carries per-repo diagnostic notes. Envelope `schema_version` remains `1` — no downstream consumer needs to update its schema knowledge.

## Diagnostics you'll see and what to do about them

| Warning | Meaning | Action |
|---|---|---|
| `No consumption reports discovered for {repo}` | The repo has the workflow deployed but it hasn't published yet, OR the workflow isn't deployed there. | Confirm the workflow is in the repo's resolved profile (`gh-aw-fleet list`) and check its Actions tab for failed runs. |
| `Included in-progress report from {repo} ({date}). Totals may be partial.` | In `--trailing` or `--since` mode, an in-progress report contributed to the sum. | If you need finalized numbers, narrow the window to exclude today, or wait for the run to finish. |
| `Run artifact for {repo} (run #N on {date}) is past the ~90-day run-log retention window.` | The discussion exists but the underlying run was garbage-collected by the platform. | Long-trend questions beyond 90 days are out of scope for this command (FR-024). Use the discussion's own prose summary if available, or shorten the window. |
| `Discussion #N on {repo} contains no actions/runs/{id}/agentic_workflow link` | An expected marker is missing — usually because the upstream workflow's report format drifted. | File against the upstream workflow; one drifted report does not block the whole rollup. |

## Mutual-exclusion errors

```bash
gh-aw-fleet consumption --latest --trailing 7d
```

→ exit code 1, error message (rendered by cobra's `MarkFlagsMutuallyExclusive`):

```text
Error: if any flags in the group [latest trailing since] are set none of the others can be; [latest trailing] were all set
```

```bash
gh-aw-fleet consumption --by tier
```

→ exit code 1, error message:

```text
Error: --by value "tier" invalid: expected one of repo, profile, cost-center, workflow
```

## What this command never does

- **Push, commit, or write** anywhere (FR-002). It's read-only end-to-end.
- **Cache** discovery or fetch results between invocations (FR-022). Each run is fresh.
- **Enforce** any budget, spending cap, or alarm (FR-023). Budget enforcement is in [GitHub's spending controls](https://github.com/settings/billing/spending_limit).
- **Parse** the discussion's rendered markdown table (FR-024). All numeric reporting goes through the structured artifact JSON, not the prose.

## What's coming later

- `--by tier` (grouping on `Profile.Tier` — the field exists, the axis just isn't wired in v1)
- Parallel discovery across repos (Assumptions §7) — for large fleets where serial latency starts to bite.
- A reliable cost field (currently nil-until-populated per Decision 6 because the upstream `aw_info.json` `cost` field is undocumented; once upstream stabilizes it, cost-aware ranking moves from informational to authoritative).
