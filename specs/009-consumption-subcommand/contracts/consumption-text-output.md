# Contract: `gh-aw-fleet consumption` text-mode output

**Feature**: 009-consumption-subcommand
**Plan**: [../plan.md](../plan.md)
**Renderer**: `cmd/consumption.go` using `text/tabwriter` (matching `cmd/list.go` at `tabPadding = 2`).

## Stderr breadcrumb

Identical to `list` (`cmd/list.go:32`):

```text
  (loaded fleet.json + fleet.local.json)
```

— or `(loaded fleet.json)` / `(loaded fleet.local.json)` depending on which files are present. Emitted via `fmt.Fprintf(cmd.ErrOrStderr(), ...)`. Text consumers expect it; JSON consumers redirect stderr.

## Stdout layout — primary table

Two stacked tables separated by one blank line. Primary table header depends on `--by`:

### `--by repo` (default)

```text
REPO                      API_CALLS  SAFE_WRITES  COST     REPORTS
rshade/example-paid       4827       31           $12.45   7
rshade/example-minimal    612        4            -        7
rshade/example-spotty     1100       7            -        3
```

### `--by profile`

```text
PROFILE          API_CALLS  SAFE_WRITES  COST     REPORTS
default          6539       42           $12.45   17
security-plus    2104       7            $8.91    7
```

### `--by cost-center`

```text
COST_CENTER      API_CALLS  SAFE_WRITES  COST     REPORTS
platform-eng     4827       31           $12.45   7
data-platform    1100       7            -        3
<unset>          612        4            -        7
```

Note `<unset>` — distinguishable from a real cost-center name; repos lacking a `cost_center` value land here (FR-015).

### `--by workflow`

```text
WORKFLOW                    API_CALLS  SAFE_WRITES  COST     REPORTS
issue-triage                3210       18           $8.91    7
ci-doctor                   2104       9            -        7
api-consumption-report      805        4            -        21
```

## Stdout layout — top-burners footer

After one blank line:

```text
TOP 10 BURNERS:
WORKFLOW                    RUNS  API_CALLS  AVG_DURATION  COST
issue-triage                156   3210       42.7s         $8.91
ci-doctor                   84    2104       31.2s         -
api-consumption-report      21    805        18.4s         -
```

When fewer than ten distinct workflows exist (FR-017): header still reads `TOP 10 BURNERS:` and the table lists exactly as many rows as exist.

## Column-rendering rules

| Field | Render | Why |
|---|---|---|
| `API_CALLS`, `SAFE_WRITES`, `RUNS`, `REPORTS` | Decimal integer, no thousand-separators | Matches `cmd/list.go`'s `%d` formatting |
| `COST` populated | `$%.2f` (e.g. `$12.45`) | Two-decimal USD, matches the upstream `api-consumption-report` rendering |
| `COST` absent | `-` (dash) | Matches `orDash` helper at `cmd/list.go:70` for the unset-string case |
| `AVG_DURATION` | `%.1fs` (e.g. `42.7s`) | Concise; matches the upstream report's per-row style |
| Cost-center value `""` | `<unset>` (under `--by cost-center` only) | Distinguishable from a real cost-center named `unset` because of the angle brackets; per FR-015 |
| Key column header | Uppercase axis name (`REPO`, `PROFILE`, `COST_CENTER`, `WORKFLOW`) | Matches `cmd/list.go`'s `REPO\tPROFILES\tTIERS\t...` header style |

## Diagnostic output

Warnings emitted by the rollup are surfaced to stderr in human-readable form before the table renders (so they're visible above the data, not buried after it):

```text
[warn] No consumption reports discovered for rshade/example-no-reports — the api-consumption-report workflow is either not deployed to this repo or has not yet produced a daily report.
[warn] Included in-progress report from rshade/example-spotty (2026-05-13). Totals for this repo may be partial.
[warn] Run artifact for rshade/example-old (run #4517 on 2026-02-08) is past the ~90-day run-log retention window.
```

Format: `[warn] {message}` on a single line. Multi-line messages join with spaces. Routed via the project's existing `zerolog`-based stderr surface (mirroring `cmd/deploy.go`'s warning emission).

## Exit codes

- `0` — rollup succeeded, even when individual repos contribute zero reports and warnings are emitted (FR-010 — "no data" is not a failure).
- `1` — config load failed (mirrors `list`), flag validation failed (`--by` value unknown, `--trailing` value malformed, more than one temporal-mode flag set), or every fleet repo failed to enumerate. Treat as "the command itself broke" not "no data found."
- The exit code surface is the same as `list`. There is no exit code "partial success" — operators read warnings.

## What this contract does NOT specify

- The exact alignment of the tabwriter columns: `text/tabwriter` aligns dynamically based on content width, so the example tables above are illustrative, not byte-exact. Tests assert column headers and row content, not pixel-level alignment.
- The order of warning emission: implementation may sort by repo name for stability, but the test only asserts that all expected warnings appear, not their relative order.
- The behavior of `--output json` — that is the contract in `consumption-output.json`.
