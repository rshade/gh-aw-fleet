# `gh aw logs --json` schema (captured on v0.79.2)

Real `--json` output captured for issue #108 to ground
[#103 "Source AIC from `gh aw logs --json`"](https://github.com/rshade/gh-aw-fleet/issues/103).
Captured 2026-06-10 with `gh aw` **v0.79.2**.

## Fixtures

| File | Source | Shape |
| --- | --- | --- |
| `logs_all_failures.json` | `gh aw logs --json -c 5` in `rshade/gh-aw-fleet` | 5 runs, **all `conclusion: "failure"`** — no `aic`, `episodes: null` |
| `logs_with_aic.json` | `gh aw logs --json --repo rshade/finfocus -c 8` | 3 success + 1 failure — **real `runs[].aic` + `summary.total_aic`** |

## Top-level shape

```jsonc
{
  "summary": { /* aggregate rollup */ },
  "runs":    [ /* per-run records — NON-UNIFORM, see below */ ],
  "episodes": null,                 // null in practice (see note)
  "edges":    null,                 // null in practice
  "observability_insights": [ /* reliability/actuation/execution insight objects */ ],
  "logs_location": ".github/aw/logs" // genericized in fixtures (was an absolute local path)
}
```

## `summary` (the clean fleet-rollup source)

| Field | Type | Notes |
| --- | --- | --- |
| `total_runs` | int | |
| `total_aic` | number | **Sum of AI Credits.** Present only once any run carries `aic`; absent on the all-failure capture. |
| `total_tokens` | int | `0` when no run reported token usage. |
| `total_turns` | int | |
| `total_action_minutes` | int | GitHub Actions minutes. |
| `total_errors` / `total_warnings` | int | |
| `total_episodes` / `high_confidence_episodes` | int | |
| `total_github_api_calls` | int | Present in the rich capture. |
| `engine_counts` | object | e.g. `{"copilot": 2}`. Present in the rich capture. |

## `runs[]` — **non-uniform** by `classification`

All runs carry: `run_id`, `number`, `workflow_name`, `workflow_path`, `status`,
`conclusion`, `classification`, `duration`, `action_minutes`, `created_at`,
`started_at`, `updated_at`, `url`, `event`, `branch`, `head_sha`,
`display_title`, `github_api_calls`.

- `classification: "baseline"` runs add **`aic`** (when successful) but omit engine/token detail.
- `classification: "normal"` runs additionally carry **`token_usage`**, **`turns`**,
  `agent`, `engine`, `engine_id`, `repository`, `organization`, `ref`, `sha`,
  `actor`, `run_attempt`, `event_name`, `avg_time_between_turns`.

| Field | Type | Notes |
| --- | --- | --- |
| `aic` | number | **AI Credits for the run.** Present on successful runs; **key is absent/null on failures.** |
| `token_usage` | int | Only on `classification: "normal"` runs; `null`/absent otherwise. **Note: `token_usage`, not `tokens`.** |
| `turns` | int | Only on `normal` runs. |
| `conclusion` | string | `success` \| `failure`. Failures carry **no** `aic`. |

## Grounding notes for #103 (assumptions adjudicated)

- ✅ **`runs[].aic` exists** (float AI-Credits) — confirmed: `125.75499`,
  `137.49432`, `260.16342`. This is the per-run actual-spend signal #103 wants.
  **But the key is absent on failed runs**, so consumers must treat it as
  nil-until-present.
- ✅ **`summary.total_aic` exists** (`523.41273`) — the simplest fleet-rollup source.
- ❌ **`episodes` / `edges` are `null` in practice** — observed `null` on *both*
  repos, including the one with successful runs. #103's assumed
  `.episodes[].total_aic` path does **not** materialize; source AIC from
  `runs[].aic` / `summary.total_aic` instead.
- ⚠️ **`cost` does not appear at all** under the Copilot AI-Credits billing model
  (consistent with the existing consumption-rollup finding that USD `cost` is
  structurally nil under Copilot). **AIC is the real denominator; USD cost is not.**
- ⚠️ `gh aw logs --json` **downloads run artifacts** (slow; writes under
  `logs_location`). A fleet aggregator should bound with `-c` and tolerate the
  on-disk side effect.
