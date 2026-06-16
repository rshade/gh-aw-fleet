# Implementation Plan: Renovate Config Conflict Scanner (Advisory)

**Branch**: `012-renovate-conflict-scanner` | **Date**: 2026-06-14 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/012-renovate-conflict-scanner/spec.md`

## Summary

Add a fourth read-only advisory scanner to the slice-006 security registry that
inspects a managed repo's clone for a Renovate configuration and warns when that
config lacks the two rules the fleet needs to keep Renovate from fighting
`gh aw upgrade`-managed pins: (A) disabling updates to the `gh-aw-actions` package
family, and (B) excluding the generated `*.lock.yml` files. Each missing rule emits
one `LOW`-severity `Finding` carrying a copy-pasteable remediation block; an
unparseable config emits one `INFO` finding. No config present → silence.

**Technical approach**: implement the `security.Scanner` interface in a new
`internal/fleet/security/renovate.go`, reading repo-root / `.github/` config paths
directly (`os.ReadFile`, mirroring `structural.go`'s `scanFile` — not
`walkWorkflows`, which only walks `.github/workflows/`). Tolerate JSON5/comments via
`hujson.Standardize()` (already an approved direct dependency — no new deps).
Register the scanner in `defaultScanners()` and map its rule IDs in
`diagCodeForRuleID()`. Because findings flow through the existing
`security.Run → SecurityFindings → {stderr, JSON warnings, PR section}` pipeline,
**no changes are needed in `deploy.go` / `sync.go` / `upgrade.go` / `cmd/`** beyond
the registry + diag-code wiring. Detection is intent-based (FR-012): a rule counts
as present if *any* config rule disables the gh-aw-actions package family or the
lock-file glob, regardless of exact syntax.

## Technical Context

**Language/Version**: Go 1.26.4 local toolchain (module declares `go 1.25.8` compatibility)
**Primary Dependencies**: `github.com/tailscale/hujson` (existing approved direct dep — JSON5-tolerant parse); stdlib `encoding/json`, `os`, `path/filepath`, `strings`. **No new third-party dependencies.**
**Storage**: N/A — pure read of files already present in the work-dir clone; no on-disk scanner state, no cache, no baseline. Findings are transient on `DeployResult`/`SyncResult`/`UpgradeResult`.
**Testing**: `go test ./internal/fleet/security/...` with table-driven tests + fixtures under `internal/fleet/security/testdata/security/` (new `renovate.json` / `.renovaterc*` fixtures); full gate via `make ci` (fmt-check, vet, lint, test).
**Target Platform**: Linux / macOS developer + CI environments running the `gh-aw-fleet` CLI.
**Project Type**: Single Go module — CLI orchestrator (`cmd/` + `internal/fleet/...`).
**Performance Goals**: Negligible — at most a handful of `os.ReadFile` calls per clone, no network. Far under the constitution's 5-minute command ceiling.
**Constraints**: Strictly read-only (FR-009); never blocks/errors a deploy (FR-007); never panics on malformed input (Scanner contract → INFO finding); no JSON envelope `schema_version` bump (additive findings only, FR-014).
**Scale/Scope**: Typical fleet ≤10 repos × 1 Renovate config each; one new ~150-line scanner file plus fixtures and tests.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Evaluated against Constitution v1.1.0.

| Principle | Verdict | Notes |
|-----------|---------|-------|
| **I. Thin-Orchestrator Code Quality** | ✅ PASS | A read-only advisory inspection of a config file, identical in shape to the existing slice-006 scanners (gitleaks, structural-frontmatter, actionlint). It does **not** re-implement Renovate's logic — it does not evaluate, resolve presets, or rewrite anything; it checks for the *presence of intent* and advises. `go build`/`go vet` stay clean; new file kept focused (<300 lines). |
| **II. Testing Standards** | ✅ PASS | Build-green + the established security-package unit-test pattern (table-driven + `testdata/` fixtures). The scanner is exercised within the existing `deploy`/`sync`/`upgrade` dry-run surface (advisory findings appear in dry-run output). |
| **III. UX Consistency** | ✅ PASS | Advisory and non-blocking — no new mutation, so the three-turn pattern does not apply. Findings reuse the existing finding pipeline; remediation is embedded in `Finding.Remedy` / `Finding.Message` (copy-pasteable block), consistent with the "actionable remediation" expectation. |
| **IV. Performance** | ✅ PASS | A few local file reads; no network, no cache needed. |
| **Third-Party Dependencies** | ✅ PASS | Reuses `github.com/tailscale/hujson` (already approved). No `go.mod` `require()` change → **no constitution amendment required.** |
| **Declarative Reconcile Invariants** | ✅ PASS | Read-only; touches no `fleet.json`/`fleet.local.json`; no git operations; no signing path. Does not modify any managed-repo file (FR-009). |

**Result**: All gates pass. No Complexity Tracking entries required.

## Project Structure

### Documentation (this feature)

```text
specs/012-renovate-conflict-scanner/
├── plan.md              # This file (/speckit-plan output)
├── spec.md              # Feature spec (/speckit-specify output)
├── research.md          # Phase 0 output — Renovate schema + decisions
├── data-model.md        # Phase 1 output — entities & finding mapping
├── quickstart.md        # Phase 1 output — how to build/test/exercise
├── contracts/
│   └── scanner-contract.md   # Phase 1 output — observable scanner contract & stable IDs
├── checklists/
│   └── requirements.md  # Spec quality checklist (already complete)
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

```text
internal/fleet/security/
├── renovate.go              # NEW — renovateScanner: probe, parse (hujson), intent-detect, emit findings
├── renovate_test.go         # NEW — table-driven tests over the fixtures below
├── constants.go             # EDIT — add rule IDs (ruleIDRenovate*) + rulePrefixRenovate
├── finding.go               # EDIT — register newRenovateScanner() in defaultScanners(); map rule IDs in diagCodeForRuleID()
└── testdata/security/renovate/   # EDIT — add Renovate fixtures (one subdir per case):
    ├── correct/renovate.json                 # both rules present (no findings)
    ├── missing-gh-aw-actions/renovate.json   # Rule A absent → 1 LOW finding
    ├── missing-lockfile/renovate.json        # Rule B absent → 1 LOW finding
    ├── missing-both/renovate.json            # neither rule → 2 LOW findings
    ├── disabled/renovate.json                # root enabled:false → no findings
    ├── equivalent-forms/renovate.json        # alternate syntax achieving both disables (no findings)
    ├── comments/renovate.json5               # JSON5 comments + trailing commas (parsed OK)
    └── malformed/renovate.json               # unparseable → 1 INFO finding

internal/fleet/fleetdiag/diag.go   # EDIT — add the single DiagSecurityRenovate code
internal/fleet/diagnostics.go      # EDIT — mirror that one code in the alias block

renovate.json                      # OPTIONAL parallel self-fix (fleet's own repo) — out of scanner acceptance
```

**Structure Decision**: Single Go module; the feature lives almost entirely in the
existing `internal/fleet/security/` package, following the slice-006 scanner
pattern. The only cross-package touch is the single additive diag code in the
`fleetdiag` leaf package (mirrored by the `internal/fleet` alias block). No new
package, no new command, no new flag.

## Phase 0: Outline & Research

All spec `[NEEDS CLARIFICATION]` markers were resolved during `/speckit-specify`
(FR-011 → `LOW`; FR-012 → intent-based matching). Phase 0 therefore focuses on the
one knowledge area the implementation depends on: the Renovate configuration schema
and which alternate expressions count as an equivalent "disable" for FR-012. See
[research.md](./research.md).

## Phase 1: Design & Contracts

- [data-model.md](./data-model.md) — the parsed Renovate config shape we read, the
  two conflict-rule definitions, and how each maps to a `Finding`.
- [contracts/scanner-contract.md](./contracts/scanner-contract.md) — the observable
  contract: inputs → findings, the new stable rule IDs + diag codes, severity
  assignment, and the canonical remediation blocks (the wire/UX contract).
- [quickstart.md](./quickstart.md) — build, test, and dry-run exercise instructions.
- Agent context: the `<!-- SPECKIT START -->`…`<!-- SPECKIT END -->` plan reference
  in `CLAUDE.md` is retargeted to this plan.

## Complexity Tracking

No constitution violations — section intentionally empty.
