# CLI Contract: `--budget`

Extends the `gh-aw-fleet consumption` command (009). Additive, read-only.

## Flag

```text
--budget <AIC>   Highlight rollup rows whose AI-credit spend exceeds this
                 per-row ceiling. Read-only: marks rows only — never caps,
                 halts, alerts, or changes the exit code. Applies to the
                 active --by axis and the TOP BURNERS footer. (default: unset)
```

- Type: float (cobra `Float64Var`).
- Optional. Absent ⇒ no annotation; output byte-identical to pre-feature behavior.
- Supplied-ness is detected with `cmd.Flags().Changed("budget")` because `0` is a valid
  ceiling (flags every row with positive spend) and cannot double as "unset".
- Composes with every `--by`, `--source`, and temporal flag; no new mutual-exclusion rules.

## Semantics

- A row is **over budget** iff its AIC is present (non-nil) and strictly greater than the
  ceiling. Equal-to-ceiling and absent-AIC rows are **not** over budget.
- The ceiling is in AI credits (AIC), the rollup's native billing unit — not USD.

## Exit codes

| Condition | Exit |
|-----------|------|
| Any number of rows over budget (including all rows) | 0 |
| No rows over budget | 0 |
| `--budget` negative | non-zero (input validation) |
| `--budget` non-numeric | non-zero (cobra flag parse) |

The number of breaching rows NEVER affects the exit code (FR-010/FR-011).

## Examples

```bash
# Flag any repo burning more than 50 AIC in the latest report
gh-aw-fleet consumption --budget 50

# By cost-center, trailing 30 days, over 200 AIC
gh-aw-fleet consumption --by cost-center --trailing 30d --budget 200

# Everything with any spend (zero ceiling)
gh-aw-fleet consumption --budget 0

# Machine-readable: read which groups breached
gh-aw-fleet consumption --by workflow --budget 25 --output json
```
