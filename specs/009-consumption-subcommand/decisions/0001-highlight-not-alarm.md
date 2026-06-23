# Decision 0001: Budget Highlighting Is Not an Alarm

## Status

Accepted for `017-consumption-budget`.

## Context

The 009 consumption subcommand specification requires that `gh-aw-fleet consumption`
MUST NOT enforce a spending cap, budget alarm, or hard limit. Feature
`017-consumption-budget` adds `--budget <AIC>`, which marks rows whose already-aggregated
AIC exceeds an operator-supplied ceiling.

## Decision

`--budget` is a read-only highlight, not an alarm or enforcement mechanism.

The feature only annotates output the operator explicitly pulled:

- Text output adds an `OVER` marker column.
- JSON output adds `result.budget` and per-row `over_budget` fields when a ceiling is supplied.
- The command exit code remains independent of breach count.

It does not push any external signal, write local or remote state, open issues or comments,
halt deploy/sync/upgrade behavior, retry work, cap spend, or constrain fleet operations.
Malformed flag input remains normal usage validation and may return non-zero; budget breaches
do not.

## Consequences

The 009 "no alarm / no enforcement" requirement remains intact. Operators can use
`--budget` to spot spend hotspots during a budget review, while actual budget enforcement
continues to live outside this tool in platform spending controls or a future explicitly
scoped feature.
