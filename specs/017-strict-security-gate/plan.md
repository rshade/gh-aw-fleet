# Implementation Plan: Strict Security Gate

**Branch**: `017-strict-security-gate` | **Date**: 2026-06-22 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/017-strict-security-gate/spec.md`

## Summary

Add an opt-in `--strict` flag to `deploy`, `sync`, and `upgrade` that promotes
HIGH-severity Layer 1 security findings from advisory warnings to blocking
failures. The existing scanner remains unchanged: strict mode consumes the same
`security.Finding` slice already surfaced on stderr, in JSON `warnings[]`, and in
PR bodies. When strict mode finds at least one blocking HIGH finding, the command
writes all findings to `findings.json` at the work-dir clone root, preserves the
clone for inspection, returns a non-zero actionable error, and stops before any
commit, push, or PR creation.

The implementation is a small policy layer around the existing scanner output:
add a `SecurityOpts` grouping to `DeployOpts`, `SyncOpts`, and `UpgradeOpts`; add
the Cobra `--strict` flag to the three commands; add a shared strict-gate helper
that counts `SeverityHigh` findings excluding `promptinj:` rule IDs; and call the
helper immediately after `security.Run` populates the result and before any
mutation gate. No scanner rules, finding ordering, output schema versions, or
compile-strict behavior change.

## Technical Context

**Language/Version**: Go 1.26.4 local toolchain; `go.mod` records the same floor.  
**Primary Dependencies**: Existing `github.com/spf13/cobra` v1.10.2 for flag wiring, existing `github.com/rs/zerolog` v1.x for stderr warnings, existing `internal/fleet/security`; stdlib `encoding/json`, `errors`, `fmt`, `os`, `path/filepath`, `strings`. No new third-party dependencies.  
**Storage**: No persistent config or cache. On strict abort only, write clone-root `findings.json` as a JSON array of all `security.Finding` values from that run; the file is a failure breadcrumb in the preserved work-dir clone.  
**Testing**: Unit tests for the strict predicate, prompt-injection carve-out, breadcrumb writer, command flag propagation, JSON/text warning behavior, and `deploy`/`sync`/`upgrade` abort placement using existing seams and temp dirs. Full gate: `make ci` (fmt-check, vet, lint, test).  
**Target Platform**: Linux / macOS developer and CI environments running the `gh-aw-fleet` CLI.  
**Project Type**: Single Go module CLI orchestrator (`cmd/` plus `internal/fleet/...`).  
**Performance Goals**: Negligible overhead beyond the scanner that already runs. Strict evaluation is O(number of findings); breadcrumb write is one small local JSON write on failure. No network fanout and no extra scanner pass.  
**Constraints**: Strict is opt-in per invocation only; never persisted to `fleet.json` / `fleet.local.json`; gate fires in dry-run and apply; abort occurs before commit/push/PR; findings are rendered before the command returns the strict error; HIGH `promptinj:` findings remain advisory; `cmd.SchemaVersion` and `fleet.SchemaVersion` do not change; `--strict` does not affect `gh aw compile --strict` resolution.  
**Scale/Scope**: Per-repo gate over typical fleets of <=10 repos and <=20 workflows each. `upgrade --all --strict` processes repos serially and stops at the first blocked repo, preserving the blocked clone.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Evaluated against Constitution v1.3.0.

| Principle | Verdict | Notes |
|-----------|---------|-------|
| **I. Thin-Orchestrator Code Quality** | PASS | The feature does not rewrite workflow markdown and does not re-implement scanner detection. It adds a small policy decision over the existing `security.Finding` slice and one JSON breadcrumb writer. Existing upstream orchestration (`gh aw`, `git`, `gh`) remains delegated. New exported identifiers (`SecurityOpts`, strict error type if exported) require godoc. |
| **II. Testing Standards** | PASS | The gate is pure enough for focused unit tests, and command paths can be covered without live PR creation by using existing seams and dry-run mode. Before implementation is complete, `make ci` is required. Any live `--apply` validation remains subject to explicit user approval. |
| **III. User Experience Consistency** | PASS | Dry-run remains the default. In apply mode strict aborts before branch/commit/push/PR, preserving the three-turn mutation pattern. The abort message is actionable and findings are still emitted on the existing stderr/JSON surfaces before the error is returned. |
| **IV. Performance Requirements** | PASS | The gate adds one in-memory scan over findings and, only on failure, one local JSON write. No new network calls, no cache invalidation, and no parallelism concerns. |
| **Declarative Reconcile Invariants** | PASS | Strict is invocation state, not fleet state. The feature does not mutate `fleet.json` / `fleet.local.json`, does not bypass signing, and does not introduce any direct operator git commands. |
| **Third-Party Dependencies** | PASS | No new direct dependency. The implementation uses stdlib JSON/file APIs and existing project packages. |
| **Documentation Impact** | PASS | User-facing CLI flags are added, so README and the Starlight reconcile docs must be updated. Operator skills that show deploy/upgrade flows should mention the strict option. |

**Result**: All gates pass. No Complexity Tracking entries required.

## Documentation Impact

- **Surfaces touched**: Cobra help for `deploy`, `sync`, and `upgrade`; `README.md` reconcile/quickstart usage; `docs/src/content/docs/reconcile.md`; `skills/fleet-deploy/SKILL.md`; `skills/fleet-upgrade-review/SKILL.md`.
- **Update planned**: yes. The feature adds visible CLI flags and changes failure behavior when those flags are present, so human-facing docs and operator skill flows must explain strict versus compile-strict and the `findings.json` breadcrumb.
- **Hidden surfaces**: none. `__schema` will expose the new flags through the existing hidden schema generator automatically; it remains deliberately undocumented.

## Project Structure

### Documentation (this feature)

```text
specs/017-strict-security-gate/
├── plan.md
├── spec.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   └── strict-gate-contract.md
└── tasks.md             # Phase 2 output (/speckit-tasks, not created by /speckit-plan)
```

### Source Code (repository root)

```text
cmd/
├── deploy.go              # add --strict flag; pass SecurityOpts into fleet.Deploy
├── deploy_test.go         # flag/help and strict error surface coverage
├── sync.go                # add --strict flag; pass SecurityOpts into fleet.Sync
├── sync_test.go           # sync strict propagation and warning coverage
├── upgrade.go             # add --strict flag; pass SecurityOpts into fleet.Upgrade/UpgradeAll
├── upgrade_test.go        # single-repo and --all strict fail-fast coverage
├── findings.go            # no behavior change expected; warning emission remains shared
├── output.go              # no schema-version change; ensure strict errors appear as hints/failures
└── schema_test.go         # update schema/help expectations for new flags if covered

internal/fleet/
├── deploy.go              # DeployOpts.Security; call gate after security.Run, before dry-run/apply split
├── deploy_test.go         # strict abort before compile/commit/PR; breadcrumb and clone-preservation tests
├── sync.go                # SyncOpts.Security; direct gate for clean/prune-only paths; propagate Deploy gate
├── sync_test.go           # dry-run, apply, prune-only, and missing-workflow strict cases
├── upgrade.go             # UpgradeOpts.Security; call gate after security.Run, before compile/PR/no-change return
├── upgrade_test.go        # dry-run/apply/--all strict cases
├── security_gate.go       # NEW: SecurityOpts, StrictSecurityError, BlockingFindings, EvaluateStrictSecurityGate
├── security_gate_test.go  # NEW: predicate, promptinj carve-out, breadcrumb JSON, error message
└── result_json.go         # no schema change; existing security_findings behavior retained

internal/fleet/security/
├── finding.go             # no scanner-output behavior change
└── constants.go           # optionally add local prompt-injection prefix only if shared in package

README.md
docs/src/content/docs/reconcile.md
skills/fleet-deploy/SKILL.md
skills/fleet-upgrade-review/SKILL.md
AGENTS.md                 # managed Spec Kit plan reference
```

**Structure Decision**: Keep the gate in `internal/fleet`, not in
`internal/fleet/security`, because it is command policy over scanner results rather
than detection logic. The security package continues to mean "produce findings";
the fleet package decides whether a command invocation treats findings as advisory
or blocking.

## Phase 0: Outline & Research

The spec contains no unresolved `NEEDS CLARIFICATION` markers. Phase 0 resolves
implementation placement and compatibility details for the opt-in strict gate. See
[research.md](./research.md).

## Phase 1: Design & Contracts

- [data-model.md](./data-model.md) defines the strict gate decision, security
  options, strict error/breadcrumb, and how the existing `Finding` shape is consumed.
- [contracts/strict-gate-contract.md](./contracts/strict-gate-contract.md) defines
  the CLI flags, blocking predicate, abort/error behavior, breadcrumb format, and
  command-specific semantics.
- [quickstart.md](./quickstart.md) documents runnable validation scenarios.
- Agent context: the managed Spec Kit block in `AGENTS.md` is retargeted to this plan.

## Post-Design Constitution Check

All Phase 1 artifacts preserve the pre-design decisions:

| Principle | Verdict | Notes |
|-----------|---------|-------|
| **I. Thin-Orchestrator Code Quality** | PASS | The contract keeps scanner logic separate from command policy and avoids new abstractions beyond `SecurityOpts` plus a focused gate helper. |
| **II. Testing Standards** | PASS | Quickstart requires focused tests plus `make ci`; live `--apply` remains manual-approval only. |
| **III. User Experience Consistency** | PASS | Contracts require findings to render before abort and require clone preservation on strict failures, matching failure-breadcrumb conventions. |
| **IV. Performance Requirements** | PASS | No extra network or scanner pass introduced. |
| **Documentation Impact** | PASS | User-facing docs and relevant skills are explicitly listed for update. |

## Complexity Tracking

No constitution violations or justified complexity exceptions.
