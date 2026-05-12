# Implementation Plan: Billing Metadata Fields

**Branch**: `007-billing-metadata-fields` | **Date**: 2026-05-10 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/007-billing-metadata-fields/spec.md`

## Summary

Add two paired optional metadata fields to the fleet schema and surface both in
`gh-aw-fleet list` text and JSON output:

- `Profile.Tier` (`json:"tier,omitempty"`) — advisory cost-tier label on profile
  definitions (`minimal | standard | premium` recommended; not enforced).
- `RepoSpec.CostCenter` (`json:"cost_center,omitempty"`) — free-form
  budget-attribution string on repo entries.

Both fields are additive — neither bumps `fleet.SchemaVersion` (on-disk format)
nor `cmd.SchemaVersion` (JSON envelope wire contract). Both are purely
declarative metadata: no validation, no enforcement, no enum, no registry
lookup. They are prerequisite group-by keys for the planned `gh-aw-fleet
consumption` subcommand.

The `list` command's text mode adds a parallel `TIERS` column (1:1 with the
existing `PROFILES` column) and a `COST_CENTER` column (with the existing `-`
placeholder when unset). JSON mode adds `profile_tiers` (a non-null
`map[string]string` keyed by profile name) and `cost_center` (always-present
string, empty when unset) to each `ListRow`. Tracks #54 (tier) and #55
(cost_center).

## Technical Context

**Language/Version**: Go 1.25.8 (per `go.mod`).
**Primary Dependencies**: `github.com/spf13/cobra` v1.10.2 (CLI), `github.com/rs/zerolog` v1.35.1 (stderr structured logging), `gopkg.in/yaml.v3` v3.0.1 (frontmatter parsing — unchanged on this feature path), `encoding/json` (stdlib). **No new third-party dependencies** — within the approved set under Constitution v1.1.0 § Third-Party Dependencies.
**Storage**: N/A. Pure read/parse of `fleet.json` / `fleet.local.json`. No persistent state outside the existing JSON files. Round-trip serialization must remain byte-identical (SC-006).
**Testing**: `go test ./...` (existing unit-test pattern in `internal/fleet/list_result_test.go`, `cmd/output_test.go`); `go run . list` for end-to-end sanity; `make ci` (`fmt-check vet lint test`) is the local gate.
**Target Platform**: Linux/macOS/Windows CLI binary; `go build ./...` clean across platforms.
**Project Type**: Single-binary Go CLI (`cmd/` for cobra wiring, `internal/fleet/` for business logic).
**Performance Goals**: Read/listing operation; well within Constitution Principle IV's 5-minute ceiling. `BuildListResult` is O(repos × profiles) and stays so — adding two field reads per row is constant-time per element.
**Constraints**: Constitution invariants (no dependency churn, additive schema only, dry-run-by-default — `list` stays read-only per FR-014). The `default` profile in `fleet.json` MUST stay byte-identical to `profiles/default.json` (existing hard invariant per CLAUDE.md).
**Scale/Scope**: Schema change touches `internal/fleet/schema.go` (2 fields), `internal/fleet/list_result.go` (2 fields + map population), `cmd/list.go` (2 columns), `fleet.json` + `profiles/default.json` (6 tier annotations), tests, and docs (CLAUDE.md/AGENTS.md, two skills). No new files in `internal/`; one new `contracts/` doc.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Per Constitution v1.1.0:

- **I. Thin-Orchestrator Code Quality**. PASS. No new orchestration surface;
  pure schema + render addition. No subprocess fanout introduced. `go build` /
  `go vet` clean is preserved by adding only typed struct fields and tabwriter
  format-string changes. Exported fields will gain godoc comments per the
  CLAUDE.md "Code self-documentation" section.
- **II. Testing Standards**. PASS. The change is non-mutating (`list` is
  read-only, FR-014), so no `--apply` dry-run obligation applies. Existing unit
  tests in `list_result_test.go` will be extended; one new test asserts the
  `tiers` map renders as `{}` not `null` when no profile carries a tier (FR-007
  edge case). `make ci` is the gate.
- **III. UX Consistency (Three-Turn Mutation Pattern)**. PASS BY EXEMPTION.
  This pattern governs mutating commands. `list` is read-only. The three-turn
  pattern does not apply.
- **IV. Performance**. PASS. No new I/O. No subprocess calls. Two extra struct
  fields per `ListRow` and one tiny map allocation per row.
- **Declarative Reconcile Invariants**. PASS. `fleet.json` and
  `fleet.local.json` remain the source of truth; this change adds two optional
  fields the loader silently accepts when absent (FR-003). The
  byte-identical-mirror invariant for the `default` profile is honored
  explicitly by FR-012 — both files receive matching tier annotations in this
  change.
- **Third-Party Dependencies**. PASS. **No additions to `go.mod`'s
  `require()` block.** All work uses existing approved direct deps + stdlib.
- **Development Workflow**. PASS. PR will include `make ci` evidence and a
  before/after `go run . list` paste. CLAUDE.md/AGENTS.md gain a one-paragraph
  description of both new fields per FR-015.

**No violations to justify.** Complexity Tracking section below stays empty.

## Project Structure

### Documentation (this feature)

```text
specs/007-billing-metadata-fields/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/
│   ├── list-output.json     # Phase 1 output: JSON envelope shape for `list`
│   └── list-text-output.md  # Phase 1 output: tabwriter text shape for `list`
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created here)
```

### Source Code (repository root)

This is a single-binary Go CLI; no `tests/` tree separate from packages
(`*_test.go` lives next to the code under test). Files touched by this feature:

```text
internal/
├── fleet/
│   ├── schema.go             # add Profile.Tier; add RepoSpec.CostCenter (json tags w/ omitempty)
│   ├── list_result.go        # add ListRow.Tiers + ListRow.CostCenter; populate in BuildListResult
│   └── list_result_test.go   # extend coverage: tier mapping, cost_center, empty-map JSON
cmd/
├── list.go                   # add TIERS and COST_CENTER columns to tabwriter header + body
└── output_test.go            # if any envelope assertions touch ListRow shape, extend

# Data files (in same change to honor FR-012 byte-identical-mirror invariant):
fleet.json                    # add "tier": "<value>" to each of the 6 profiles
profiles/default.json         # add "tier": "standard" to mirror the default profile

# Documentation (FR-015):
AGENTS.md                     # add a paragraph on both fields under a fleet-schema subsection
skills/fleet-onboard-repo/SKILL.md   # mention cost_center as an optional onboarding field
skills/fleet-build-profile/SKILL.md  # mention tier in the Step 2 (sources/pins) flow
```

**Structure Decision**: Single-binary Go CLI, layout already established by the
project (cmd/ + internal/fleet/). No new packages; no test-tree restructure.
This feature fits entirely within existing files except for the spec
artifacts under `specs/007-billing-metadata-fields/`.

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

*No violations. Section intentionally empty.*
