---
name: fleet-budget-review
description: Aggregate per-repo api-consumption-report output across the fleet and recommend where to look for cost concentration. Use this skill whenever the user says "what's our fleet spending", "where is the cost concentrated", "which workflows are burning credits", "consumption rollup", "budget review", "cost by profile", "cost by cost-center", "monthly fleet usage", "what's the top burner", "any spend hotspots", or asks about Copilot-credit usage across multiple repos. Also trigger when the user asks "should we cut anything from `<profile>`" or "is `<repo>` pulling its weight" — both questions resolve to a consumption rollup grouped on the relevant axis. The skill runs `gh-aw-fleet consumption` with the right temporal and group-by flags, reads the structured output, and frames the result as a budget conversation (where the spend is, what's driving it, what to consider next). Do not trigger for deploying or bumping workflows (that's `fleet-deploy` / `fleet-upgrade-review`), for evaluating which upstream workflows to adopt (that's `fleet-eval-templates`), or for onboarding a new repo (`fleet-onboard-repo`).
---

# Fleet Budget Review

Aggregate `api-consumption-report` output across the fleet and surface where the cost is concentrated — by repo, by profile, by cost-center, or by individual workflow.

## Why this skill exists

`gh-aw-fleet consumption` is read-only by design (FR-002). It does not enforce budgets, set caps, or alarm — those live in [GitHub's spending controls](https://github.com/settings/billing/spending_limit). What it *does* is answer the operator's budget question: "where is the fleet's Copilot-credit usage going, and which workflows are driving it?"

The default data source (`--source logs`) uses the GitHub Actions logs API. Discovery enumerates each repo's agentic workflows (those compiled to `.lock.yml` files) via `gh api repos/.../actions/workflows`; data comes from `gh aw logs --json` per workflow, summing AI credits (`summary.total_aic`). This path needs no deployed `api-consumption-report` workflow — the rollup is decoupled from any deployed artifact. For backwards compatibility, a legacy `--source artifacts` path is available: discovery walks the repo's `audits`-category discussions for the `<!-- gh-aw-tracker-id: api-consumption-report-daily -->` marker, and data comes from the discussion-referenced workflow run's `aw_info.json` + `run_summary.json` artifacts (now mostly deprecated under the Copilot AI-Credits model, where cost is structurally absent). No caching — each invocation re-discovers from scratch (FR-022). See [`docs/finops.md`](../../docs/finops.md) for the two-layer cost model (this aggregate rollup vs. the optional per-run `cost-tracker`) and the AIC-vs-USD caveats.

This skill exists because a raw `gh-aw-fleet consumption` invocation produces a tabwriter rollup, but the operator's question is rarely "show me a table." It's "should we trim the security-plus opt-ins on the lower-traffic repos?" — a question the table answers but doesn't *frame*. The skill picks the right flags, reads the JSON, and frames the conclusion.

## When to use

- User wants a fleet-wide spend snapshot (current state, trailing window, or since a fixed date).
- User wants to attribute consumption to a budget axis (profile, cost-center, workflow).
- User wants to identify "top burners" — individual workflows driving the most calls or cost.
- User is doing a periodic budget review or quarterly cost-center reconciliation.
- User wants to compare spend before / after a profile change.

Skip for:

- Deploying a workflow to a repo — `fleet-deploy`.
- Auditing pin drift / action versions — `fleet-upgrade-review`.
- Evaluating which upstream workflows to adopt — `fleet-eval-templates`.
- Onboarding a new repo into the fleet — `fleet-onboard-repo`.
- Enforcing a budget cap — out of scope; that's GitHub's spending-controls UI, not this tool (FR-023).
- Reading historical reports older than ~90 days — the upstream run-log retention window cuts the data off; the rollup degrades gracefully but the answer isn't there (FR-024).

## The flow

Read-only single-turn pattern — there is no dry-run/apply distinction because nothing mutates. The "judgment" step is choosing the right `--by` axis for the user's question and framing the output as actionable.

### Step 1 — pick the temporal mode

Three mutually-exclusive flags. Default is `--latest`.

| Flag | When to use |
|---|---|
| `--latest` (default) | Snapshot question — "where are we right now?" One report per repo. |
| `--trailing 7d` | Window question — "what did we spend this week?" Sums all reports in the trailing N-day window per repo. |
| `--since 2026-04-01` | Period question — "what have we spent since the new fiscal quarter started?" Sums all reports on or after the date. |

Days are the only supported unit on `--trailing` — `7d`, `30d`, `90d`. Sub-day (`12h`) and weeks (`2w`) are not accepted; the upstream report cadence is daily so sub-day precision is meaningless.

`--since` accepts `YYYY-MM-DD`. UTC. The window opens at midnight UTC of the input date and runs to now.

### Step 2 — pick the group-by axis

```bash
gh-aw-fleet consumption --by <axis>
```

Four axes:

- `--by repo` (default) — one row per repo. Answers "which repos are most expensive?"
- `--by profile` — one row per profile the fleet uses. Multi-profile repos contribute **additively to every profile group** (research.md Decision 5 — sums across profile rows will exceed the fleet total because of double-counting). Answers "what does each profile cost when everything that uses it runs?"
- `--by cost-center` — one row per declared `cost_center` value, plus a literal `<unset>` bucket for repos without one (FR-015). Each repo contributes to exactly one bucket. Answers "where does spend attribute by org budget?"
- `--by workflow` — pivot away from repos entirely; one row per distinct workflow name, summed across every repo running it. Answers "which workflows are the top burners?"

Any value outside the four-axis set is a hard error — cobra rejects with `--by value "X" invalid: expected one of repo, profile, cost-center, workflow`.

### Step 3 — invoke

```bash
gh-aw-fleet consumption [repo...] [--latest|--trailing Nd|--since YYYY-MM-DD] --by <axis>
```

**Scope to specific repos:** pass one or more `owner/name` args to limit the rollup to just those repos; omit them for the whole fleet. This is the way to drill into *one* repo's per-workflow spend — `gh-aw-fleet consumption rshade/finfocus --by workflow` answers "which workflows are burning credits in finfocus?", whereas a bare `--by workflow` sums each workflow across every repo running it. Unknown repo names are a hard error that lists the offenders (validated before any network call).

Add `--output json` if you want to pipe through jq. The envelope is the standard fleet shape (`schema_version: 1`, `command: "consumption"`, `result.groups[]`, `result.top_burners[]`, `warnings[]`).

The command prints a stderr breadcrumb (`(loaded fleet.json + fleet.local.json)`) and structured warnings; stdout carries the rollup table and the `TOP 10 BURNERS:` footer.

### Step 4 — read the diagnostics

The rollup may emit per-repo warnings on stderr. Two patterns to expect under the default `--source logs`:

- **`No consumption reports discovered for <repo>`** — the repo has no agentic workflows deployed, or no workflows have run yet. Check `gh-aw-fleet list <repo>` to see which workflows are resolved for this repo. If none are present, add a profile that includes agentic workflows. If workflows are present, wait 24h for the first run to complete and report.
- **`Included in-progress report from <repo> (<date>). Totals for this repo may be partial.`** — only appears under `--trailing` / `--since`. An in-progress workflow run (still executing) contributed a partial data snapshot. Re-run after the run completes for finalized totals, or narrow the time window to exclude today.

Warnings are non-fatal. The command always exits 0 when discovery + fetch succeed for at least one repo — "no data found" is not a failure (FR-010).

**Legacy `--source artifacts` notes:** If using the deprecated artifacts path, expect an additional warning: **`Run artifact for <repo> (run #N on <date>) is past the ~90-day run-log retention window.`** This occurs when the underlying workflow-run artifacts were garbage-collected by the platform. Long-trend questions beyond ~90 days are out of scope (FR-024); shorten the window or switch to `--source logs`.

### Step 5 — frame the answer

The raw table is rarely the answer. Pick the framing that matches the question:

- **Snapshot question** ("how are we doing?"): lead with the fleet total AI credits (sum of the `AIC` column for `--by repo`), then call out the top 3 repos by credit usage. The `TOP 10 BURNERS:` footer names the highest-traffic individual workflows by AIC — quote 2-3 of them.
- **Cost-concentration question** ("where do we trim?"): use `--by profile` or `--by cost-center` and call out the heaviest row. Note any `<unset>` bucket — those repos aren't attributed to a budget owner and that's usually a tagging gap to fix before any real trimming decision.
- **Workflow-specific question** ("is `<workflow>` worth it?"): use `--by workflow`, pull out the named workflow's row, and compare its `AIC` / `COST` to the next-most-expensive workflow. Numbers in isolation rarely change anyone's mind; a comparison ("`workflow-X` is 3× the AIC of the next-most-expensive workflow `Y`") does.
- **Trend question** ("are we trending up?"): the command doesn't store historical state, so you can't directly compare windows. Run two invocations (`--trailing 7d` and `--trailing 14d`) and infer the delta (last 7d ÷ first 7d ≈ 2 means flat; >2 means growth, <2 means decline). Flag that this is approximate — daily variance can be large.

### Step 6 — recommend (only if user asked)

If the user asks "what should I do?":

- For an unattributed `<unset>` cost-center bucket: recommend adding `cost_center` annotations on those repos in `fleet.local.json`. The annotation is advisory — the loader silently accepts its absence — so it's a documentation fix, not a code change.
- For a workflow burning disproportionate calls: name the workflow, name the profile(s) it ships in (`gh-aw-fleet list` shows resolved workflow sets per repo), and suggest excluding it on the lowest-value-per-call repos via `RepoSpec.ExcludeFromProfiles`. Don't propose removing it from the profile entirely unless the user signals that's on the table.
- For a repo with no reports (under `--source logs`): name the repo and check `gh-aw-fleet list <repo>` to see if any agentic workflows are deployed. If none, add a profile that includes agentic workflows. If workflows exist but haven't run yet, wait 24h for the first execution. (Under `--source artifacts`, the repo needs `observability-plus` and its daily workflow run.)
- For retention-expired data: tell the user the limit and stop. The platform doesn't expose this data; pretending the rollup can answer is worse than saying "out of scope."

Never apply changes from this skill. Recommendations are read-only — operator follows up with the `fleet-onboard-repo` or `fleet-deploy` skill (or a hand edit) if they want to act.

## Pre-spend forecast

`gh-aw-fleet forecast` is the pre-spend twin of `consumption`. Where `consumption` reports what the fleet *spent*, `forecast` projects what it *will* spend, using `gh aw forecast --json` per repo.

**Single-turn flow** (no dry-run/apply since it's read-only):

```bash
gh-aw-fleet forecast [--period week|month] [--by repo|profile|cost-center|tier] [repo...]
```

Read the projected AIC table and frame as: "fleet is projected to spend X AIC (= $Y) over the next week/month." A complete budget conversation = observed rollup (from `consumption`) + forward projection (from `forecast`).

**Flag reference:**

- `--period week|month` (default `week`) — maps to upstream `--days 7|30`.
- `--by repo|profile|cost-center|tier` (default `repo`) — note that `tier` is forecast-only (not available in `consumption`). Same axes as `consumption` except `tier` groups by the `Profile.Tier` field.
- `[repo...]` positional args — scope to named `owner/name` repos.
- `--output json` — inherited persistent flag for pipe-friendly output.

**Reading the output:**

- `PROJECTED_AIC` is the authoritative point estimate (summed projected AI credits).
- `P10/P50/P90` is an *advisory* Monte Carlo confidence band — approximate (summing per-workflow percentiles is not statistically exact); treat as indicative spread, not a guarantee.
- A cold group (`cold=true`, `SAMPLED 0`, dashes in band columns) means no run history yet — not "free". Wait for runs to accumulate.
- `PROJECTED_COST` = `PROJECTED_AIC × $0.01`.

**Minimum gh-aw CLI version:** v0.79.2 required (checked automatically before any repo call).

**Advisory note:** `forecast` does no Monte Carlo math itself — it sums the upstream point estimates and advisory bands from `gh aw forecast`. The tool's value is the fleet-wide fan-out and group-by aggregation.

## Invariants

- **Read-only.** This skill never invokes `--apply` (consumption has no such flag — read-only by design). Recommendations route through other skills that handle mutation correctly.
- **No caching.** Each invocation re-discovers. If the user runs the rollup twice in a session and gets slightly different numbers, that's because in-progress reports flipped to finalized between invocations — not a bug. Surface this when totals shift unexpectedly.
- **Multi-profile additivity is the documented semantic.** Don't apologize for double-counting under `--by profile` — it's intentional (FR-014, research.md Decision 5). Operators learn to sum across profile rows only when they want a fleet total, not when comparing profile costs.
- **AI credits and cost are nil-until-positive.** If `aic` / `cost` fields are zero or absent, the rollup renders `-` for those columns (FR-018, Decision 6). Don't treat the dash as "this repo costs zero dollars" — it means "no AIC data available." Under `--source logs`, all fields are populated from agentic run summaries when available; under `--source artifacts`, the cost field is structurally nil under the Copilot AI-Credits model (that path predates the AIC schema). Frame conclusions around API_CALLS when AIC is sparse.
- **The `<unset>` cost-center bucket is a real signal, not noise.** When the operator asks "where is my spend attributed?", an `<unset>` row means "you haven't told the tool, and I'm not going to invent an answer."
- **Don't volunteer raw JSON unless asked.** The text-mode tabwriter is the operator-facing surface; the JSON envelope is for downstream piping. Show the table; only break out JSON if the user pipes through jq or asks for the wire format.

## What "success" looks like

A concise budget-frame paragraph + the relevant table, organized like:

```text
Fleet consumption (trailing-7d, by cost-center):

  COST_CENTER     API_CALLS  SAFE_WRITES  AIC      COST    REPORTS
  platform-eng    33,789     147          523.41   $5.23   14
  data-platform   8,402      31           98.67    $0.99   7
  <unset>         2,108      9            -        -       7

Frame:
  - Platform-eng is 5.3× the AI credits of data-platform — expected given the
    repo count (2× repos, 2× per-repo activity and higher-cost workflows).
  - 7 reports under <unset> means two repos still aren't attributed to
    a budget owner. Recommend annotating before the quarterly review.
  - Top burners: issue-triage (523.41 AIC, $5.23 cost) leads — but only
    ~10% of the cost-center total, so cost is broad-based, not
    concentrated. Trimming would mean broad changes, not surgical ones.
```

If the user asks for a specific axis or window, run the relevant invocation and frame accordingly. Don't volunteer every axis — pick the one that answers the question.

## Examples

### Example 1 — quick snapshot

```text
User: how are we doing on fleet spend?

You:
  - Run `gh-aw-fleet consumption` (default --latest, --by repo).
  - Read the table. Note the top 3 repos by API_CALLS, the top 2-3
    burner workflows from the footer, any warnings.
  - Frame: "Latest snapshot — N repos with reports, top spender is X
    at $Y. Top burner is workflow-Z. Two repos have no reports yet
    (observability-plus not deployed)."
```

### Example 2 — quarterly cost-center review

```text
User: I need to attribute Q1 spend by cost-center for the budget review.

You:
  - Pick --since for the calendar-Q1 boundary: `--since 2026-01-01`.
  - Pick --by cost-center.
  - Run `gh-aw-fleet consumption --since 2026-01-01 --by cost-center`.
  - Read table. Note the <unset> bucket explicitly.
  - Frame as a budget summary; if <unset> is non-trivial, recommend
    annotating the repos before next quarter.
```

### Example 3 — workflow trimming decision

```text
User: is the agentics issue-triage workflow worth keeping?

You:
  - Run `gh-aw-fleet consumption --trailing 30d --by workflow`.
  - Pull out the issue-triage row.
  - Cross-reference against the next-most-expensive workflow to give
    a relative magnitude.
  - Also note: `gh-aw-fleet list` shows which profile ships
    issue-triage and which repos resolve it.
  - Frame: "issue-triage is N% of fleet API calls over the last 30
    days. If you want to scale it back, the lightest move is excluding
    it on the lowest-value repos via ExcludeFromProfiles, not removing
    from the profile entirely."
  - Stop. Wait for the user's call.
```

### Example 4 — empty fleet

```text
User: what's our consumption?

You:
  - Run `gh-aw-fleet consumption`.
  - All repos emit "No consumption reports discovered" warnings.
  - Report: "No consumption reports are available — the
    api-consumption-report workflow isn't deployed to any repo in
    the fleet yet. Add `observability-plus` to a repo's profile list
    in fleet.local.json and re-run after its first daily run."
  - Stop. Do not invent numbers.
```

## Extended usage

For machine-readable output, pass `--output json`. The result envelope is documented in `specs/009-consumption-subcommand/contracts/consumption-output.json`. Note that `cost` is omitted via `omitempty` when nil (Decision 6) — downstream jq filters can `select(.cost != null)` to skip rows with no cost signal.

For deeper context on the discovery + parsing layers (e.g., when adding a new diagnostic), read `specs/009-consumption-subcommand/contracts/discussion-discovery.md` and `contracts/run-artifact-payload.md`. The fixture tree under `internal/fleet/testdata/consumption/` shows the exact upstream shapes consumed.
