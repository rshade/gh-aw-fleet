# Data Model: Billing Metadata Fields

**Feature**: 007-billing-metadata-fields
**Date**: 2026-05-10

Three existing types are extended. No new types are introduced. All field additions are optional and additive on both on-disk and envelope contracts.

## On-disk schema (fleet.json / fleet.local.json / profiles/default.json)

### Profile (extended)

Existing `Profile` struct in `internal/fleet/schema.go`. The `Tier` field is added; all other fields unchanged.

```go
type Profile struct {
    Description string               `json:"description,omitempty"`
    Tier        string               `json:"tier,omitempty"`        // NEW
    Sources     map[string]SourcePin `json:"sources"`
    Workflows   []ProfileWorkflow    `json:"workflows"`
}
```

| Field | Type | JSON key | Required | Description |
| ----- | ---- | -------- | -------- | ----------- |
| `Tier` | `string` | `tier` | No (advisory) | Cost-tier label. Recommended `minimal`, `standard`, or `premium`. Free-form — no closed enum, no load-time validation. Empty string is preserved on read but dropped on write via `omitempty`. |

**Validation rules**: none. Per FR-010, any operator-chosen value is preserved verbatim.

**State transitions**: N/A — purely descriptive metadata.

**Round-trip behavior**: `Tier == ""` is equivalent to the field being absent. Configs that don't declare `tier` continue to parse and render unchanged (FR-003).

### RepoSpec (extended)

Existing `RepoSpec` struct in `internal/fleet/schema.go`. The `CostCenter` field is added; all other fields unchanged.

```go
type RepoSpec struct {
    Profiles            []string          `json:"profiles"`
    CostCenter          string            `json:"cost_center,omitempty"` // NEW
    Engine              string            `json:"engine,omitempty"`
    ExtraWorkflows      []ExtraWorkflow   `json:"extra,omitempty"`
    ExcludeFromProfiles []string          `json:"exclude,omitempty"`
    Overrides           map[string]string `json:"overrides,omitempty"`
}
```

| Field | Type | JSON key | Required | Description |
| ----- | ---- | -------- | -------- | ----------- |
| `CostCenter` | `string` | `cost_center` | No | Free-form budget-attribution label. Maps 1:1 to a GitHub cost-center name; the fleet tool does not validate that the named cost center actually exists. |

**Validation rules**: none (FR-011, FR-016). The field receives no special handling based on which file it appears in.

**State transitions**: N/A.

**Round-trip behavior**: `CostCenter == ""` is equivalent to absent. Existing repos without the field continue to load and list unchanged.

### Config (unchanged)

`Config` (`internal/fleet/schema.go:14-20`) is unchanged. The existing `Repos map[string]RepoSpec` and `Profiles map[string]Profile` automatically carry the new fields through the map values.

### Existing schema-version constant

`fleet.SchemaVersion` in `internal/fleet/schema.go` remains at `1`. Both field additions are additive on the on-disk format and do not require a version bump (FR-004).

## JSON envelope (`gh-aw-fleet list --output json`)

### ListRow (extended)

Existing `ListRow` struct in `internal/fleet/list_result.go`. Two fields are added.

```go
type ListRow struct {
    Repo         string            `json:"repo"`
    Profiles     []string          `json:"profiles"`
    ProfileTiers map[string]string `json:"profile_tiers"`  // NEW
    Engine       string            `json:"engine"`
    Workflows    []string          `json:"workflows"`
    Excluded     []string          `json:"excluded"`
    Extra        []string          `json:"extra"`
    CostCenter   string            `json:"cost_center"`    // NEW
}
```

| Field | Type | JSON key | Always present? | Description |
| ----- | ---- | -------- | --------------- | ----------- |
| `ProfileTiers` | `map[string]string` | `profile_tiers` | Yes (empty map when no tiers set) | Maps profile name → tier value for this row's profiles. Keys are a subset of `Profiles` — only profiles whose underlying `Profile.Tier` is non-empty appear here. Empty map when none. |
| `CostCenter` | `string` | `cost_center` | Yes (empty string when unset) | Mirrors `RepoSpec.CostCenter`. Always emitted in the envelope per FR-008. |

**Population in `BuildListResult`** (`internal/fleet/list_result.go`):

- `ProfileTiers`: iterate `spec.Profiles`; for each profile name, look up `cfg.Profiles[name].Tier`; if non-empty, insert into the map. Always initialize as `map[string]string{}` (not nil) so JSON marshals as `{}`.
- `CostCenter`: copy `spec.CostCenter` directly (empty string when unset is the contract).

**Non-nil contract**: Maintains the existing FR-009 invariant — `Profiles`, `Workflows`, `Excluded`, `Extra` are all non-nil empty slices when empty. `ProfileTiers` follows the same rule with `map[string]string{}`.

### ListResult (unchanged)

`ListResult` in `internal/fleet/list_result.go` is unchanged. Embeds `[]ListRow` which automatically carries the new fields.

### Existing envelope-version constant

`cmd.SchemaVersion = 1` (per `cmd/output.go`, referenced in spec FR-009) remains unchanged. Both `profile_tiers` and `cost_center` are additive on the envelope contract.

## Tabwriter (text-mode) output

### Header

```text
REPO    PROFILES    TIERS    ENGINE    WORKFLOWS    EXCLUDED    EXTRA    COST_CENTER
```

- `TIERS` is **new** and is inserted between `PROFILES` and `ENGINE`. Its slice positions correspond 1:1 with the same row's `PROFILES` slice positions.
- `COST_CENTER` is **new** and is appended at the end of the row.

### Row format

Today (`cmd/list.go`):

```go
fmt.Fprintf(tw, "%s\t%v\t%s\t%d\t%v\t%d\n",
    r, spec.Profiles, orDash(cfg.EffectiveEngine(r)), len(resolved),
    orEmpty(spec.ExcludeFromProfiles), len(spec.ExtraWorkflows))
```

After this feature:

```go
fmt.Fprintf(tw, "%s\t%v\t%v\t%s\t%d\t%v\t%d\t%s\n",
    r,
    spec.Profiles,                                  // unchanged %v on []string
    tiersForRow(spec.Profiles, cfg.Profiles),       // NEW: parallel []string
    orDash(cfg.EffectiveEngine(r)),
    len(resolved),
    orEmpty(spec.ExcludeFromProfiles),
    len(spec.ExtraWorkflows),
    orDash(spec.CostCenter))                         // NEW column
```

`tiersForRow` is a new private helper in `cmd/list.go` that walks the row's profile names in order and returns a `[]string`:

- For each profile whose underlying `cfg.Profiles[name].Tier` is non-empty, append the tier value.
- For each profile with an empty tier, append `"-"` (the existing unset-string placeholder shared with `Engine` via `orDash` in `cmd/list.go`).
- **Special case**: when **every** position would be `"-"` (no profile in the row has a tier), return `[]string{}` so `%v` formats as `[]` — matching today's slice-empty convention for `Excluded` and `Extra`.

This logic prevents `%v`'s ambiguous double-space rendering for mixed tiered/untiered rows (`["standard","","premium"]` would format as `[standard  premium]` with a confusing gap).

**Rendering examples** (cells shown bracketed):

| Repo state                                      | PROFILES cell             | TIERS cell             |
|-------------------------------------------------|---------------------------|------------------------|
| One profile, tier set                           | `[default]`               | `[standard]`           |
| Two profiles, both tiered                       | `[default quality-plus]`  | `[standard premium]`   |
| Three profiles, only middle one tiered          | `[a default c]`           | `[- standard -]`       |
| One profile, no tier                            | `[legacy-custom]`         | `[]`                   |
| Two profiles, neither tiered                    | `[a b]`                   | `[]`                   |

The helper is a pure function easily unit-testable in `cmd/list_test.go` (new file) without a tabwriter dependency.

## Relationships

```text
Config
 ├── Profiles: map[string]Profile     (Profile gains Tier)
 └── Repos:    map[string]RepoSpec    (RepoSpec gains CostCenter)
                  │
                  └── Profiles: []string  (list of profile names)
                       │
                       └── (each name → Config.Profiles[name].Tier
                                       populates ListRow.ProfileTiers)

BuildListResult(cfg)
 └── []ListRow                         (ListRow gains ProfileTiers + CostCenter)
        ├── ProfileTiers populated from cfg.Profiles[name].Tier
        └── CostCenter copied from cfg.Repos[repo].CostCenter
```

## Identity & uniqueness

- Profile names are unique within `Config.Profiles` (existing invariant; new field doesn't change this).
- Repo names are unique within `Config.Repos` (existing invariant).
- `ProfileTiers` keys in a `ListRow` are unique by construction (it's a Go map).

## Data volume

- Typical fleet: ≤10 repos, ≤6 profiles. `ProfileTiers` per row carries 0–3 entries in practice. Memory and serialization overhead are negligible.

## Migration

None required. Both fields are `omitempty` on the on-disk format; missing fields parse as empty, equivalent to unset (FR-003).
