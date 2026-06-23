# Quickstart: Over-Budget Highlighting

## What it does

`gh-aw-fleet consumption --budget <AIC>` flags rollup rows whose AI-credit spend exceeds a
ceiling, so spend hotspots stand out without manual scanning. It is **read-only**: it marks
rows only — it never caps, halts, alerts, or changes the exit code.

## Try it

```bash
# Highlight repos over 50 AIC in the latest report
gh-aw-fleet consumption --budget 50

# Pivot by cost-center over the last 30 days, ceiling 200 AIC
gh-aw-fleet consumption --by cost-center --trailing 30d --budget 200

# Show everything with any spend
gh-aw-fleet consumption --budget 0

# Machine-readable
gh-aw-fleet consumption --by workflow --budget 25 --output json | jq '.result.groups[] | select(.over_budget)'
```

Over-budget rows show `!` in a trailing `OVER` column (text) or `"over_budget": true` (JSON).
The applied ceiling is echoed as `result.budget` in JSON.

## Rules at a glance

- **Over budget** = AIC present **and** strictly greater than the ceiling. Equal-to-ceiling
  and rows with no AIC (`-`) are never flagged.
- Unit is **AIC** (AI credits), the native billing unit — not USD.
- No `--budget` ⇒ output is exactly as before (purely additive, opt-in).
- Exit code is **always 0** for any number of breaches; only a **negative** or
  **non-numeric** `--budget` exits non-zero (input validation).

## Verify the implementation

```bash
make fmt && make vet && make lint && make test     # or: make ci
go run . consumption --budget 50                   # eyeball the OVER column
go run . consumption --budget 50 --output json | jq '.schema_version'   # must stay 1
go run . consumption --budget -1; echo "exit=$?"   # must be non-zero
go run . consumption --budget 0; echo "exit=$?"    # must be 0
```

### Acceptance checks (map to spec)

- [ ] Rows with `AIC > budget` marked on **all four** `--by` axes (US1, US2; FR-002/FR-003).
- [ ] Equal-to-ceiling and nil-AIC rows **not** marked (FR-002/FR-009).
- [ ] No `--budget` ⇒ text and JSON identical to pre-feature (FR-006; SC-003).
- [ ] With `--budget`, JSON carries additive `budget` + `over_budget`; `schema_version == 1` (FR-007/FR-008; SC-004).
- [ ] Exit code 0 under any breach count; non-zero only for malformed input (FR-010/FR-011; SC-005).
- [ ] Decision record present under `specs/009-consumption-subcommand/decisions/` (FR-014; SC-008).
- [ ] `skills/fleet-budget-review/SKILL.md` documents `--budget` (FR-015).
- [ ] No new dependency; `make ci` green (SC-006/SC-007).
