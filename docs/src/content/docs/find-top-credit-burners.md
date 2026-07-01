---
title: Find your top credit burners
description: Identify which repositories and workflows consume the most AI credits across the fleet.
---

`gh-aw-fleet consumption` rolls up AI-credit (AIC) usage across the fleet from
`gh aw logs`. Change the `--by` axis and the window to answer different "where did
the credits go?" questions. It is read-only and needs no deployed reporting
workflow.

## Rank repositories

Start with the per-repo rollup (the default axis):

```bash
gh-aw-fleet consumption --by repo
```

Each row is a repository with its API calls, safe writes, AIC, and derived USD
(`AIC × $0.01`). The highest AIC rows are your biggest spenders.

## Rank individual workflows

To find the single workflows burning the most, group by workflow. The output
includes a `TOP 10 BURNERS` footer ranked by AIC:

```bash
gh-aw-fleet consumption --by workflow
```

## Drill into one repository

Scope the rollup to one repo and break it down by workflow to see where that
repo's spend concentrates:

```bash
gh-aw-fleet consumption you/your-repo --by workflow
```

## Narrow the time window

`consumption` defaults to the most-recent data per repo (`--latest`). To measure
a period instead, use a trailing window or a start date (the three are mutually
exclusive):

```bash
gh-aw-fleet consumption --trailing 7d --by workflow    # last 7 days
gh-aw-fleet consumption --since 2026-06-01 --by repo   # since a date
```

## Flag rows over a ceiling

Add `--budget <AIC>` to mark rows whose AIC strictly exceeds a ceiling with a `!`
in a trailing `OVER` column — useful for spotting concentration at a glance:

```bash
gh-aw-fleet consumption --by repo --budget 500
```

## Export for a cost system

Emit the JSON envelope to feed the numbers into a spreadsheet or cost tool:

```bash
gh-aw-fleet consumption --by cost-center --output json
```

## See also

- [Consumption and FinOps](/gh-aw-fleet/consumption/) — the two-layer cost model and why
  AIC, not USD, is the true unit.
- [Fleet Overview](/gh-aw-fleet/overview/) — drift, health, and cost joined into one
  dashboard.
