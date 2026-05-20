# Phase 1 Data Model: Compile Workflows with --strict on Public Repos

**Date**: 2026-05-17
**Spec**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md) | **Research**: [research.md](./research.md)

This document enumerates the on-disk and in-memory entities introduced or extended by this feature. There is no relational store — all state lives in JSON-on-disk (`fleet.local.json` / `fleet.json`) and Go structs in `internal/fleet/`.

## E1 — `RepoSpec.CompileStrict` (extension to existing entity)

**Location**: `internal/fleet/schema.go`, on the existing `RepoSpec` struct.

**Field**:

| Name | Go Type | JSON Tag | Required | Default | Notes |
|------|---------|----------|----------|---------|-------|
| `CompileStrict` | `*bool` | `compile_strict,omitempty` | No | `nil` | Tri-state: `nil` (auto-detect), `true` (force ON), `false` (force OFF). |

**Round-trip contract**: Absence of the field on existing `fleet.local.json` files MUST round-trip byte-identically through HuJson AST mutation. Adding the field (via `Add` or operator hand-edit) MUST preserve comments and other fields. Removing the field returns the spec to auto-detect behavior.

**Validation rules**: None at load time. Any boolean value is valid. `nil` is the canonical "not set" state.

**State transitions**: None — this is configuration, not lifecycle state.

**Relationships**: Sibling to `Profiles`, `CostCenter`, `Engine`, `ExtraWorkflows`, `ExcludeFromProfiles`, `Overrides` on the same `RepoSpec`. No cross-entity constraints.

## E2 — `CompileStrictResolution` (in-memory)

**Location**: `internal/fleet/schema.go`, returned from `Config.EffectiveCompileStrict(ctx, repo)`.

**Conceptual shape** (not a named struct in code; returned as a 3-tuple `(effective bool, source string, reason string)` — the resolver never returns an error, fail-secure semantics fold lookup failures into the `auto-fallback` source):

| Field | Type | Domain | Notes |
|-------|------|--------|-------|
| `Effective` | `bool` | `{true, false}` | The decision: whether to invoke `gh aw compile --strict`. |
| `Source` | `string` | `{"explicit", "auto-public", "auto-private", "auto-fallback"}` | Discriminant naming why `Effective` has its value. |
| `Reason` | `string` | free-form, ≤200 chars | Populated only when `Source == "auto-fallback"`. Truncated raw `error.Error()` for FR-007 log enrichment. Empty string for the other three sources. |

**Resolution algorithm** (from FR-003):

1. If `RepoSpec.CompileStrict != nil`: return `(*spec.CompileStrict, "explicit", "")`.
2. Else, call `ghRepoVisibility(ctx, repo)` (the package-level seam — wraps `gh api /repos/<owner>/<repo> --jq .visibility`):
   - Success + visibility `== "public"` → `(true, "auto-public", "")`.
   - Success + visibility `!= "public"` (private, internal, anything else) → `(false, "auto-private", "")`.
   - Error or visibility absent / non-string → `(true, "auto-fallback", truncate(err.Error(), 200))`.

**Persistence**: None — recomputed every invocation per FR-008 + Assumption #1.

## E3 — `CLIProbeOutcome` (in-memory)

**Location**: `internal/fleet/deploy.go` (and reused from `upgrade.go`).

**Conceptual shape** (returned from the probe helper as `(outcome, detail, error)`):

| Field | Type | Domain | Notes |
|-------|------|--------|-------|
| `Outcome` | `string` | `{"flag-present", "flag-absent", "probe-failed"}` | Three-way discriminant per FR-016. |
| `DetectedVersion` | `string` | Semver or empty | Populated only when probe ran successfully; result of R3 regex extraction against `gh aw --version`. Empty when version parse fails or when probe itself failed (`probe-failed`). |
| `ProbeError` | `error` | nil unless `Outcome == "probe-failed"` | Wrapped underlying `exec.Command` error for diagnostics. |

**Resolution algorithm** (FR-016, R2 from research):

1. Run `gh aw compile --help`. If exit non-zero or exec error → `("probe-failed", "", err)`.
2. If output `Contains "--strict"` → resolve `DetectedVersion` via R3, return `("flag-present", version, nil)`.
3. Else → resolve `DetectedVersion` via R3, return `("flag-absent", version, nil)`.

**Persistence**: None — invoked once per Deploy/Upgrade only when `Effective == true`.

## E4 — `DeployResult` / `UpgradeResult` Extension

**Location**: `internal/fleet/deploy.go` (`DeployResult`) and `internal/fleet/upgrade.go` (`UpgradeResult`).

**New fields on `DeployResult`**:

| Name | Go Type | JSON Tag | Default | Notes |
|------|---------|----------|---------|-------|
| `CompileStrictApplied` | `bool` | `compile_strict_applied` | `false` | `true` only when `gh aw compile --strict` was actually invoked in this run. `false` covers both "skipped because effective was false" AND "didn't reach the resolver" (e.g., early failure). |
| `CompileStrictSource` | `string` | `compile_strict_source` | `""` | One of `"explicit"`, `"auto-public"`, `"auto-private"`, `"auto-fallback"`, or `""` (no resolver call occurred — meaning early error path). Empty string is **distinct** from the four valid values; consumers MUST treat it as "n/a." |

**New fields on `UpgradeResult`**: Identical shape and semantics to the `DeployResult` additions above.

**Population rules**:

- Set both fields immediately after `EffectiveCompileStrict` returns, BEFORE the probe runs. This ensures the source is recorded even if the probe later aborts.
- `CompileStrictApplied` is updated to `true` only after the `gh aw compile --strict` subprocess returns exit 0. If compile fails, the field stays `false` and `Source` retains the resolver's value.
- The resume-from-work-dir path that already sets `MissingSecret` / `ActionsDisabled` MUST also call the resolver and set these fields, so re-invoked deploys produce the same envelope shape.

**Wire contract**: Additive — JSON consumers ignoring unknown fields are unaffected. No `cmd.SchemaVersion` bump required.

## E5 — Diagnostic Codes

**Location**: `internal/fleet/fleetdiag/diag.go` (canonical leaf-package home) with re-exports in `internal/fleet/diagnostics.go`.

| Constant Name | String Value | Triggered By | Hint Content |
|---------------|--------------|--------------|--------------|
| `DiagCompileStrictFailed` | `compile_strict_failed` | Stderr matching `"strict mode validation"` or `"strict mode requires"` (from `gh aw compile --strict`). | "Workflow violates strict-mode validation. Inspect the work-dir clone at `<path>`. To opt this repo out, set `\"compile_strict\": false` in `fleet.local.json` for `<repo>`." |
| `DiagGhAwTooOld` | `gh_aw_too_old` | Probe outcome `flag-absent`. | "Local `gh aw` version is too old (detected `<version>`; minimum `v0.68.3`). Upgrade with `gh extension upgrade aw`. To bypass, set `\"compile_strict\": false` for repos that don't need strict compile." |
| `DiagGhAwMissing` | `gh_aw_missing` | Probe outcome `probe-failed` (exec error / binary not found). | "`gh aw` extension is missing or broken. Install with `gh extension install github/gh-aw`. Underlying error: `<raw stderr>`." |

**Code-value stability**: These string values appear in the JSON envelope's `warnings[].code` field (per the existing spec 005 / spec 009 precedent) and SHOULD be treated as part of the CLI's wire contract. Renaming requires a `cmd.SchemaVersion` bump.

## Entity relationships

```text
RepoSpec ──┐
           │ (read by)
           ▼
Config.EffectiveCompileStrict(ctx, repo)
           │ produces
           ▼
CompileStrictResolution {Effective, Source, Reason}
           │ feeds
           ▼
┌──────────┴──────────────────────┐
│                                 │
▼                                 ▼
DeployResult.CompileStrict*       Probe (only if Effective==true)
                                    │
                                    ├── flag-present → invoke compile
                                    │                    │
                                    │                    ├── success → DeployResult.CompileStrictApplied=true
                                    │                    └── fail    → CollectHints(DiagCompileStrictFailed)
                                    │
                                    ├── flag-absent  → CollectHints(DiagGhAwTooOld)
                                    └── probe-failed → CollectHints(DiagGhAwMissing)
```

All entities are transient (in-memory per invocation) except `RepoSpec.CompileStrict` which is on-disk via `fleet.json` / `fleet.local.json` HuJson round-trip.
