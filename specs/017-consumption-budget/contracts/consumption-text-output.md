# Text-Output Contract: `OVER` column

Additive delta to the 009 tabwriter text output. The `OVER` column appears **only when
`--budget` is supplied**; without it, output is byte-identical to the pre-feature table.

## With `--budget 50` (axis: repo)

```text
REPO                  API_CALLS  SAFE_WRITES  AIC    COST   REPORTS  OVER
rshade/finfocus       1200       8            73.50  $0.74  1        !
rshade/gh-aw-fleet    300        2            12.00  $0.12  1
rshade/all-failures   0          0            -      -      1

TOP 10 BURNERS:
WORKFLOW                    RUNS  API_CALLS  AVG_DURATION  AIC    COST   OVER
daily-malicious-code-scan   30    900        42.5s         61.00  $0.61  !
nightly-docs                 4    120        10.0s         18.00  $0.18
```

- `OVER` cell is `!` for over-budget rows, empty otherwise.
- Trailing position keeps existing columns and alignment unchanged.
- A `-` AIC row (nil) is never marked.
- The footer uses the same `OVER` column on the same AIC ceiling.

## Without `--budget` (unchanged)

```text
REPO                  API_CALLS  SAFE_WRITES  AIC    COST   REPORTS
rshade/finfocus       1200       8            73.50  $0.74  1
rshade/gh-aw-fleet    300        2            12.00  $0.12  1

TOP 10 BURNERS:
WORKFLOW                    RUNS  API_CALLS  AVG_DURATION  AIC    COST
daily-malicious-code-scan   30    900        42.5s         61.00  $0.61
```

## Header construction

`renderConsumptionText` appends `\tOVER` to both header lines and an `OVER`-cell to each
row only when `res.Budget != nil`; the over-cell renders `"!"` when the row's `OverBudget`
is true and `""` otherwise.
