# Phase 1 Data Model: Public `pkg/fleet` Config Contract

This slice **moves** an existing, stable data model; it does not design a new one.
The entities below are the seven config-contract types relocated verbatim from
`internal/fleet/schema.go` into `pkg/fleet/config.go`, plus the `SchemaVersion`
constant. Field names, types, and JSON tags are preserved **byte-for-byte** —
the tables double as the FR-005 fidelity checklist.

## Entity relationship (composition)

```text
Config
├── Version        int
├── Defaults       Defaults
├── Profiles       map[string]Profile
│                     └── Profile
│                          ├── Sources    map[string]SourcePin
│                          └── Workflows  []ProfileWorkflow
├── Repos          map[string]RepoSpec
│                     └── RepoSpec
│                          └── ExtraWorkflows  []ExtraWorkflow
└── LoadedFrom     string   (json:"-", loader-only)
```

The set is **closed**: no field references a type outside these seven (verified
against the source). That is what lets them relocate without dragging
`Templates`, `ResolvedWorkflow`, or any network helper across the module boundary.

---

## Config — root of the wire contract

| Field | Type | JSON tag | Omit behavior (MUST preserve) |
|-------|------|----------|-------------------------------|
| `Version` | `int` | `version` | always present |
| `Defaults` | `Defaults` | `defaults,omitzero` | omitted when zero-value (Go 1.24+ `omitzero`) |
| `Profiles` | `map[string]Profile` | `profiles,omitempty` | omitted when nil/empty |
| `Repos` | `map[string]RepoSpec` | `repos` | **no omit** — encodes as `null` when nil |
| `LoadedFrom` | `string` | `-` | **never serialized**; set by `LoadConfig` only |

- **Method** (moves with the type, stays a method): `EffectiveEngine(repo string) string`
  — returns the per-repo `Engine` override if set, else `Defaults.Engine`. Pure;
  no internal/network dependency (FR-004).
- **Validation**: `LoadConfig` rejects `Version != SchemaVersion` — but that check
  lives in `internal/fleet/load.go` and **stays there** (FR-016). The contract type
  itself enforces nothing.
- **Edge cases locked by tags**: `Defaults` uses `omitzero` (NOT `omitempty`);
  `Profiles` uses `omitempty`; `Repos` has *no* omit tag (the three behaviors are
  deliberately different and must each survive — spec Edge Cases).

## Defaults — fleet-wide defaults

| Field | Type | JSON tag | Omit behavior |
|-------|------|----------|---------------|
| `Engine` | `string` | `engine,omitempty` | omitted when `""` |

## Profile — atomically-advancing workflow bundle

| Field | Type | JSON tag | Omit behavior |
|-------|------|----------|---------------|
| `Description` | `string` | `description,omitempty` | omitted when `""` |
| `Tier` | `string` | `tier,omitempty` | advisory cost label; omitted when `""` |
| `Sources` | `map[string]SourcePin` | `sources` | always present |
| `Workflows` | `[]ProfileWorkflow` | `workflows` | always present |

## SourcePin — pinned ref for a source repo within a profile

| Field | Type | JSON tag | Omit behavior |
|-------|------|----------|---------------|
| `Ref` | `string` | `ref` | always present |

## ProfileWorkflow — a workflow entry in a profile

| Field | Type | JSON tag | Omit behavior |
|-------|------|----------|---------------|
| `Name` | `string` | `name` | always present |
| `Source` | `string` | `source` | always present |
| `Path` | `string` | `path,omitempty` | omitted when `""` |

## RepoSpec — per-repo desired state

| Field | Type | JSON tag | Omit behavior |
|-------|------|----------|---------------|
| `Profiles` | `[]string` | `profiles` | always present |
| `CostCenter` | `string` | `cost_center,omitempty` | advisory; omitted when `""` |
| `Engine` | `string` | `engine,omitempty` | omitted when `""` |
| `CompileStrict` | `*bool` | `compile_strict,omitempty` | tri-state; omitted when nil |
| `ExtraWorkflows` | `[]ExtraWorkflow` | `extra,omitempty` | **omitted when empty** (drops `"extra": []`) |
| `ExcludeFromProfiles` | `[]string` | `exclude,omitempty` | **omitted when empty** (drops `"exclude": []`) |
| `Overrides` | `map[string]string` | `overrides,omitempty` | omitted when nil/empty |

> The two `omitempty` slices are why `fleet.example.json` cannot be the byte-for-byte
> golden target (research.md Decision 4): the example's `"extra": []` / `"exclude": []`
> vanish on re-marshal. `CompileStrict` is read by the *relocated* standalone
> `EffectiveCompileStrict` function in `internal/fleet`, not by anything in `pkg/fleet`.

## ExtraWorkflow — per-repo workflow not from any profile

| Field | Type | JSON tag | Omit behavior |
|-------|------|----------|---------------|
| `Name` | `string` | `name` | always present |
| `Source` | `string` | `source` | always present |
| `Ref` | `string` | `ref,omitempty` | omitted when `""` |
| `Path` | `string` | `path,omitempty` | omitted when `""` |

## SchemaVersion — constant

| Identifier | Type | Value | Notes |
|------------|------|-------|-------|
| `SchemaVersion` | untyped int const | `1` | on-disk fleet-config format version; **distinct** from `cmd.SchemaVersion` (JSON envelope). MUST NOT change (FR-011, SC-006). Re-exported in `internal/fleet` via `const SchemaVersion = pkgfleet.SchemaVersion`. |

---

## What does NOT move (stays in `internal/fleet`)

These are listed so a reviewer can confirm the boundary is correct:

- **Impure / deploy-path**: `EffectiveCompileStrict` (method → func),
  `ghRepoVisibility` (network), `truncateReason`, `effectiveCompileStrictReasonMax`,
  `CompileStrictSource{Explicit,AutoPublic,AutoPrivate,AutoFallback}`,
  `VisibilityPublic` (FR-008).
- **Catalog cache**: `Templates`, `TemplateSource`, `TemplateWorkflow`, `Evaluation`
  (FR-015).
- **Load / resolve**: `LoadConfig`, `mergeConfigs`, HuJson read path,
  `ResolveRepoWorkflows` (method → func), `ResolvedWorkflow` type (FR-016).
