# Contract: `gh-aw-fleet list` text-mode output (after this feature ships)

This contract documents the visible behavior of the default (non-JSON) text
output produced by `gh-aw-fleet list` after the billing metadata fields land.
Exact whitespace is governed by Go's `text/tabwriter` and may shift when
neighboring rows widen — the contract pins column **identity, order, and
content rules**, not pixel-exact alignment.

## Header row

```text
REPO                       PROFILES                TIERS                 ENGINE   WORKFLOWS  EXCLUDED  EXTRA  COST_CENTER
```

- `TIERS` is **new** and is inserted **between `PROFILES` and `ENGINE`**.
- `COST_CENTER` is **new** and is appended at the end of the row.
- `PROFILES` retains its column position and rendering (Go's `%v` on
  `[]string` — bracketed, space-separated). It does not change.

## Behavior rules (testable)

1. The `TIERS` cell starts with `[` and ends with `]`. Its slice positions
   correspond 1:1 with the same row's `PROFILES` cell positions.
2. For each profile whose underlying `cfg.Profiles[name].Tier` is non-empty,
   the corresponding `TIERS` slot shows the tier value verbatim.
3. For each profile with an empty tier, the corresponding `TIERS` slot shows
   `-` (the existing unset-string placeholder shared with `Engine`).
4. **Special case**: when **every** profile in the row is untiered, the
   `TIERS` cell renders `[]` (matches today's `Excluded`/`Extra` empty-slice
   convention).
5. The `COST_CENTER` cell is either the operator-supplied value or `-`. Never
   empty, never absent.
6. Column count is **stable**: two more columns than before this feature
   (`TIERS` inserted, `COST_CENTER` appended).

## Example rows

### Row 1: two profiles, both tiered, cost-center set

```text
rshade/example-paid        [default security-plus]  [standard premium]    copilot  3  []  0  platform-eng
```

Two profiles in use; both have tiers; tier values render in slice positions
matching the profile names. Cost-center column shows the operator-supplied
value verbatim.

### Row 2: one profile, tier set, cost-center unset

```text
rshade/example-minimal     [default]                [standard]            copilot  1  []  0  -
```

Single profile, single tier, slot-aligned. Cost-center column shows `-`.

### Row 3: one profile, no tier set on its underlying definition

```text
rshade/example-untyped     [legacy-custom]          []                    claude   1  []  0  -
```

Profile has no tier on its `Profile.Tier` definition; `TIERS` cell renders
`[]` (the all-untiered special case). Cost-center unset → `-`.

### Row 4: mixed — some profiles have tiers, some don't

```text
rshade/example-mixed       [default custom-legacy]  [standard -]          copilot  2  []  0  growth
```

`default` has tier `standard`; `custom-legacy` has no tier and renders as
`-` in its slot. Position-1 of `PROFILES` pairs with position-1 of `TIERS`,
etc. — the binding is by index, never by string match.

## Consumer impact

- **Human readers**: gain immediate visibility into cost framing without a
  separate command; the parallel column is greppable
  (`gh-aw-fleet list | grep premium`).
- **Shell scripts** parsing `list` text output: must be updated to expect
  two additional `\t`-delimited columns (`TIERS` inserted between PROFILES and
  ENGINE, `COST_CENTER` appended). The recommended migration is to consume
  `--output json` instead — that channel is forward-compatible by construction
  via the additive `profile_tiers` and `cost_center` fields.
- **Subagents / Claude skills** in `skills/fleet-*`: continue to use
  `--output json` (already documented in those skills); no change required.

## Backward compatibility

Per the spec (FR-003 and SC-003):

- Existing fleet configurations without `tier` or `cost_center` annotations
  produce output where every row's `TIERS` cell is `[]` and the
  `COST_CENTER` column is `-`. The output is shape-additive: existing columns
  retain their content and meaning.
- No flag is needed to opt into the new fields. They simply render when
  present in the configuration.

## What this contract does NOT pin

- Pixel-exact spacing inside each cell. `tabwriter` widens columns to fit
  the widest cell across all rows; tests should assert by token, not column
  position.
- The order of profiles within a row's `PROFILES` (and therefore `TIERS`)
  cell. Order is `RepoSpec.Profiles` declaration order — the order is stable
  across runs but is operator-controlled, not alphabetized by the tool.
