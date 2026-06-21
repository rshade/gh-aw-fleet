# Quickstart: `gh-aw-fleet forecast`

The pre-spend twin of `gh-aw-fleet consumption`. Where `consumption` reports
what the fleet *spent*, `forecast` projects what it *will* spend — so you can
catch an expensive rollout before next month's bill.

## Prerequisites

- `gh` with the `gh aw` extension **≥ v0.79.2** installed
  (`gh extension install github/gh-aw --pin v0.79.2`). Older CLIs are rejected
  with a minimum-version error before any repo call.
- A loadable `fleet.json` / `fleet.local.json` in the working directory.

## Common invocations

```bash
# Whole-fleet monthly projection, grouped by repo (default period=week, by=repo)
gh-aw-fleet forecast --period month

# Weekly projection grouped by billing tier
gh-aw-fleet forecast --period week --by tier

# Attribute projected spend to cost-centers (with the <unset> bucket)
gh-aw-fleet forecast --by cost-center

# Scope to two repos and pipe to jq
gh-aw-fleet forecast rshade/finfocus rshade/gh-aw-fleet --output json | jq '.result.groups'
```

## Flags

| Flag | Values | Default | Meaning |
|---|---|---|---|
| `--period` | `week` \| `month` | `week` | projection horizon (maps to upstream `--days 7\|30`) |
| `--by` | `repo` \| `profile` \| `cost-center` \| `tier` | `repo` | group-by axis |
| `--output` | `text` \| `json` | `text` | inherited persistent flag |
| `[repo...]` | positional | (whole fleet) | scope to named `owner/name` repos |

## Reading the output

```text
  (loaded fleet.json + fleet.local.json)
  forecast: period=month by=repo

REPO               PROJECTED_AIC  PROJECTED_COST  P10    P50      P90      SAMPLED  WORKFLOWS
rshade/finfocus    137.49         $1.37           0.00   137.49   549.98   1        1
rshade/gh-aw-fleet 0.00           -               -      -        -        0        4
```

- **PROJECTED_AIC** is the authoritative point estimate (summed projected AI
  credits). **PROJECTED_COST** is `PROJECTED_AIC × $0.01`.
- **P10/P50/P90** is an *advisory* Monte Carlo confidence band. Summing
  per-workflow percentiles is approximate — trust `PROJECTED_AIC` for
  decisions; treat the band as indicative spread.
- A row with `cold=true` (rendered `0.00` / `-` / `-`) means **no run history**,
  not "free." `SAMPLED 0` confirms it. Wait for runs to accumulate, then
  re-forecast.

## Smoke test (offline)

The aggregation layer is fully covered by offline tests over committed
fixtures — no `gh` required:

```bash
go test ./internal/fleet/ -run Forecast
go test ./cmd/ -run Forecast
```

Fixtures live in `internal/fleet/testdata/forecast/`
(`forecast_single_workflow.json`, `forecast_cold_start.json`). The `ghForecastAPI`
seam is reassigned in tests to load them; production shells `gh aw forecast`.

## Full CI gate

```bash
make ci   # fmt-check, vet, lint, test — must pass before the change is done
```

## How it relates to `consumption`

| | `consumption` | `forecast` |
|---|---|---|
| Question | "what did we spend?" | "what will we spend?" |
| Data | `gh aw logs --json` (observed runs) | `gh aw forecast --json` (Monte Carlo projection) |
| Metric | observed AIC | projected AIC (point + band) |
| Axes | repo/profile/cost-center/workflow | repo/profile/cost-center/**tier** |
| Mutates? | No | No |

Both feed the `fleet-budget-review` skill — pair an observed rollup with a
forward projection for a complete budget conversation.
