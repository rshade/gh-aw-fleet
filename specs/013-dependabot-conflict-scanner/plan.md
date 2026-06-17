# Implementation Plan: Dependabot Config Conflict Scanner (Advisory)

**Branch**: `013-dependabot-conflict-scanner` | **Date**: 2026-06-16 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/013-dependabot-conflict-scanner/spec.md`

## Summary

Add a fifth read-only advisory scanner to the slice-006 security registry — the
**sibling of the Renovate scanner (012)** — that inspects a managed repo's clone for
a Dependabot configuration and warns when a `github-actions` ecosystem update entry
does **not** ignore the `gh-aw-actions` action family. An out-of-band Dependabot bump
of `github/gh-aw-actions` rewrites the generated `*.lock.yml` files and desyncs them
from the compiler version baked into lock-file metadata (live instance:
`rshade/finfocus#1246`). Each unprotected `github-actions` entry emits one
`LOW`-severity `Finding` carrying a copy-pasteable `ignore:` block; an unparseable
config emits one `INFO` finding. No config, or a config with no `github-actions`
entry → silence.

**The one structural divergence from the Renovate sibling**: Dependabot ignores by
dependency *name* only — there is **no file-glob analog** to Renovate's `*.lock.yml`
exclusion (Rule B). So this scanner has exactly **one** conflict rule, not two, and
its remedy carries an education burden the Renovate one does not: it must state that
the lock files remain reachable by name (FR-004). The second divergence is the parse
path — Dependabot config is **YAML**, read via `gopkg.in/yaml.v3` (already an approved
direct dep), not `hujson`.

**Technical approach**: implement the `security.Scanner` interface in a new
`internal/fleet/security/dependabot.go`, probing `.github/dependabot.yml` /
`.yaml` directly (`os.ReadFile`, mirroring `structural.go`'s file-read shape — not
`walkWorkflows`). Parse with `yaml.Unmarshal` into a minimal struct (new import in the
security package — but **no new `go.mod` dependency**, so no constitution amendment).
Register the scanner in `defaultScanners()` and map its rule-ID prefix in
`diagCodeForRuleID()`. Because findings flow through the existing
`security.Run → SecurityFindings → {stderr, JSON warnings, PR section}` pipeline,
**no changes are needed in `deploy.go` / `sync.go` / `upgrade.go` / `cmd/`** beyond
the registry + diag-code wiring. Detection is intent-based (FR-012): an entry counts
as protected if its `ignore` list covers the gh-aw lineage (substring `gh-aw`) **or**
its `open-pull-requests-limit` is `0` (cannot open bump PRs), regardless of exact
syntax.

## Technical Context

**Language/Version**: Go 1.26.4 local toolchain (module declares `go 1.25.8` compatibility)
**Primary Dependencies**: `gopkg.in/yaml.v3` (existing approved direct dep — YAML parse, already used by `internal/fleet/frontmatter`); stdlib `os`, `path/filepath`, `strings`. **No new third-party dependencies.**
**Storage**: N/A — pure read of a probed config already present in the work-dir clone; no on-disk scanner state, no cache, no baseline. Findings are transient on `DeployResult`/`SyncResult`/`UpgradeResult`.
**Testing**: `go test ./internal/fleet/security/...` with table-driven tests + fixtures under `internal/fleet/security/testdata/security/dependabot/<case>/.github/dependabot.yml`; full gate via `make ci` (fmt-check, vet, lint, test).
**Target Platform**: Linux / macOS developer + CI environments running the `gh-aw-fleet` CLI.
**Project Type**: Single Go module — CLI orchestrator (`cmd/` + `internal/fleet/...`).
**Performance Goals**: Negligible — at most two `os.ReadFile` probes per clone, no network. Far under the constitution's 5-minute command ceiling.
**Constraints**: Strictly read-only (FR-009); never blocks/errors a deploy (FR-007); never panics on malformed input (Scanner contract → INFO finding); no JSON envelope `schema_version` bump (additive findings only, FR-014).
**Scale/Scope**: Typical fleet ≤10 repos × ≤1 Dependabot config each, usually one `github-actions` entry; one new ~150-line scanner file plus fixtures and tests.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Evaluated against Constitution v1.1.0.

| Principle | Verdict | Notes |
|-----------|---------|-------|
| **I. Thin-Orchestrator Code Quality** | ✅ PASS | A read-only advisory inspection of a config file, identical in shape to the existing slice-006 scanners (gitleaks, structural-frontmatter, actionlint, renovate). It does **not** re-implement Dependabot — it does not evaluate, resolve org config, or rewrite anything; it checks for the *presence of intent* and advises. `go build`/`go vet` stay clean; new file kept focused (<300 lines). |
| **II. Testing Standards** | ✅ PASS | Build-green + the established security-package unit-test pattern (table-driven + `testdata/` fixtures). The scanner is exercised within the existing `deploy`/`sync`/`upgrade` dry-run surface (advisory findings appear in dry-run output). |
| **III. UX Consistency** | ✅ PASS | Advisory and non-blocking — no new mutation, so the three-turn pattern does not apply. Findings reuse the existing finding pipeline; remediation is embedded in `Finding.Remedy` / `Finding.Message` (copy-pasteable `ignore:` block + name-only caveat), consistent with the "actionable remediation" expectation. |
| **IV. Performance** | ✅ PASS | Two local file probes; no network, no cache needed. |
| **Third-Party Dependencies** | ✅ PASS | Reuses `gopkg.in/yaml.v3` (already approved, grandfathered at v1.0.0). The security package gains a new *import* of it, but `go.mod`'s `require()` block is unchanged → **no constitution amendment required.** |
| **Declarative Reconcile Invariants** | ✅ PASS | Read-only; touches no `fleet.json`/`fleet.local.json`; no git operations; no signing path. Does not modify any managed-repo file (FR-009). |

**Result**: All gates pass. No Complexity Tracking entries required.

## Project Structure

### Documentation (this feature)

```text
specs/013-dependabot-conflict-scanner/
├── plan.md              # This file (/speckit-plan output)
├── spec.md              # Feature spec (/speckit-specify output)
├── research.md          # Phase 0 output — Dependabot schema + decisions
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
├── dependabot.go            # NEW — dependabotScanner: probe, parse (yaml.v3), intent-detect, emit findings
├── dependabot_test.go       # NEW — table-driven tests over the fixtures below
├── constants.go             # EDIT — add rule IDs (ruleIDDependabot*) + rulePrefixDependabot
├── finding.go               # EDIT — register newDependabotScanner() in defaultScanners(); map prefix in diagCodeForRuleID()
└── testdata/security/dependabot/   # NEW — Dependabot fixtures (one subdir per case, config at <case>/.github/dependabot.yml):
    ├── correct/.github/dependabot.yml                  # github-actions entry ignores gh-aw family (no findings)
    ├── missing-ignore/.github/dependabot.yml           # github-actions entry, no ignore block at all → 1 LOW
    ├── partial-unrelated-ignore/.github/dependabot.yml # github-actions entry, ignore covers an unrelated dep (actions/checkout) but not gh-aw → 1 LOW
    ├── gomod-only/.github/dependabot.yml               # only gomod ecosystem → no findings
    ├── wildcard-ignore/.github/dependabot.yml          # ignore uses github/gh-aw-actions* wildcard → no findings
    ├── pr-limit-zero/.github/dependabot.yml            # github-actions entry, open-pull-requests-limit: 0 → no findings
    ├── multiple-unprotected/.github/dependabot.yml     # two github-actions entries, both unprotected → 2 LOW
    └── malformed/.github/dependabot.yml                # unparseable YAML → 1 INFO

internal/fleet/fleetdiag/diag.go   # EDIT — add the single DiagSecurityDependabot code
internal/fleet/diagnostics.go      # EDIT — mirror that one code in the alias block

.github/dependabot.yml             # OPTIONAL parallel parity note (fleet's own — gomod-only, NOT at risk) — out of scanner acceptance
```

**Structure Decision**: Single Go module; the feature lives almost entirely in the
existing `internal/fleet/security/` package, following the slice-006 scanner pattern
established by `renovate.go` (012). The only cross-package touch is the single
additive diag code in the `fleetdiag` leaf package (mirrored by the `internal/fleet`
alias block). No new package, no new command, no new flag.

## Phase 0: Outline & Research

All spec `[NEEDS CLARIFICATION]` markers were resolved during `/speckit-specify`
(severity → `LOW`/`INFO` and matching → intent-based, both inherited from the
Renovate sibling). Phase 0 therefore focuses on the Dependabot configuration schema,
which alternate forms count as a protecting "ignore" for FR-012, and the
single-conflict-rule asymmetry. See [research.md](./research.md).

## Phase 1: Design & Contracts

- [data-model.md](./data-model.md) — the parsed Dependabot config shape we read, the
  single conflict-rule definition, and how each gap maps to a `Finding`.
- [contracts/scanner-contract.md](./contracts/scanner-contract.md) — the observable
  contract: inputs → findings, the new stable rule IDs + diag code, severity
  assignment, and the canonical `ignore:` remediation block with the name-only caveat
  (the wire/UX contract).
- [quickstart.md](./quickstart.md) — build, test, and dry-run exercise instructions.
- Agent context: the `<!-- SPECKIT START -->`…`<!-- SPECKIT END -->` plan reference
  in `CLAUDE.md` is retargeted to this plan.

## Complexity Tracking

No constitution violations — section intentionally empty.
