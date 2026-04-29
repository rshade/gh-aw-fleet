# Implementation Plan: `status` Subcommand for Drift Detection

**Branch**: `004-status-drift-detection` | **Date**: 2026-04-28 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/004-status-drift-detection/spec.md`

## Summary

Replace the `cmd/stubs.go` `newStatusCmd()` stub with a complete read-only drift-detection command that compares fleet.json's declared workflow set against each repo's actual `.github/workflows/*.md` `source:` frontmatter — without cloning. Status reuses the existing `gh api` delegation in `internal/fleet/fetch.go`, the existing `SplitFrontmatter` / `ParseFrontmatter` helpers in `internal/fleet/frontmatter.go`, and the existing `CollectHints` diagnostic layer. The new `internal/fleet/status.go` adds a `Status(...)` entrypoint plus a bounded-parallel (4–8 worker) fan-out per FR-018, and emits a `StatusResult` that fits into the JSON envelope contract introduced by spec 003. Exit code is `0` only when every queried repo is `aligned`; any drift, error, or unpinned workflow returns `1`.

## Technical Context

**Language/Version**: Go 1.25.8 (per `go.mod`).
**Primary Dependencies**: `github.com/spf13/cobra` v1.10.2 (CLI), `github.com/rs/zerolog` v1.x (stderr structured logging), `gopkg.in/yaml.v3` (frontmatter parsing — already in use), `encoding/json` (stdlib, JSON envelope). **No new third-party dependencies** (SC-006 / Constitution Principle I).
**Storage**: N/A — pure read command, no on-disk state, no cache. Output is transient to stdout.
**Testing**: `go test ./...`. Table-driven unit tests in `internal/fleet/status_test.go` for diff logic against in-memory fixtures (no network). Real-world validation via dry-run against the project's own fleet (parity with `deploy --dry-run` for the same repo per FR-002 / spec Testing Strategy).
**Target Platform**: Linux/macOS/WSL — wherever the `gh-aw-fleet` Go binary already runs. No platform-specific code paths.
**Project Type**: CLI tool (single Go module, `cmd/` + `internal/fleet/` layered as elsewhere in the repo).
**Performance Goals**: <2 s/repo when M ≤ 5 (SC-002); <20 s for a 10-repo fleet (SC-001); 4–8 worker pool repo-level parallelism (FR-018, clarification 1).
**Constraints**: Zero clones (FR-002 / SC-003); zero mutation (FR-017); strict string comparison for refs (FR-004, clarification 3); default-branch-only reads (clarification 2); per-repo failure isolation (FR-009).
**Scale/Scope**: Designed for fleets up to ~50 repos × ~20 workflows each. Beyond that, GitHub rate-limit pressure (5000 req/hr authenticated) becomes the bottleneck; the worker pool's bounded concurrency is the mitigation.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Compliance | Evidence |
|---|---|---|
| **I. Thin-Orchestrator Code Quality** | ✅ PASS | Wraps `gh api` (existing `ghAPIRaw` / `ghAPIJSON`); never re-implements GitHub content fetching, never invokes git, never clones. Reuses `SplitFrontmatter`, `ParseFrontmatter`, `SourceLayout`, `CollectHints`. New files: `cmd/status.go` (~120 LOC est.), `internal/fleet/status.go` (~200 LOC est.) — well under the 300-line refactor threshold. |
| **II. Testing Standards** | ✅ PASS | `go build`/`go vet` clean is enforced as always. Status is read-only, so the "real-world dry-run before `--apply`" rule does not apply (no `--apply` exists). Equivalent: spec mandates manual integration test against a real fleet (Testing Strategy in spec). Unit tests for the pure diff function (constitution permits but does not require unit tests; the spec's P1/P3 acceptance criteria call for them, which is additive). |
| **III. User Experience Consistency** | ✅ PASS (NOT APPLICABLE) | Three-turn pattern applies to mutating commands. Status is strictly read-only — there is no `--apply`, no PR, no commit, no branch, no clone. Diagnostic surfacing is required (FR-010) and parallels the `deploy` / `upgrade` failure-hint UX, but uses structured `Diagnostic`s constructed at the call site (`Code: repo_inaccessible`, `Fields.repo: ...`) rather than substring-matched `CollectHintDiagnostics` — see research R5 for the disambiguation rationale. No commit-message generation. |
| **IV. Performance Requirements** | ✅ PASS | Bounded-parallel fan-out across repos (FR-018) honors "I/O-bound operations on independent targets SHOULD be parallelized." No persistent cache needed — status is a live snapshot and caching contradicts its semantics. SC-001 / SC-002 targets are well under the 5-minute ceiling. |
| **Declarative Reconcile Invariants** | ✅ PASS | Reads `fleet.json` (+ `fleet.local.json`) without mutating either. No git invocations, so the gpg / `git add` / `git commit` invariants do not apply. Status reports drift against the source-pinning invariant — it enforces, rather than violates, it. |

**Result**: All gates pass. **No Complexity Tracking entries required.**

## Project Structure

### Documentation (this feature)

```text
specs/004-status-drift-detection/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   ├── cli-surface.md   # Cobra command shape, args, flags, exit codes
│   └── json-envelope.md # JSON shape for `-o json` (status-specific extension of envelope contract)
├── checklists/
│   └── requirements.md  # Quality checklist (already 16/16 from /speckit.specify)
├── spec.md              # Source spec (post-clarify, four Q&As recorded)
└── tasks.md             # Generated by /speckit.tasks (NOT this command)
```

### Source Code (repository root)

This feature touches an existing single-project Go CLI. The structural pattern (one cmd/<name>.go per Cobra command + one internal/fleet/<name>.go per feature module) is already established by `deploy`, `sync`, `upgrade`, `add`. Status follows that pattern.

```text
cmd/
├── add.go
├── deploy.go
├── hints.go
├── list.go
├── output.go
├── output_test.go
├── root.go
├── root_logging_test.go
├── root_test.go
├── status.go               # NEW — Cobra wrapper, output formatting, JSON envelope, exit code
├── status_test.go          # NEW — JSON-envelope and dispatch tests
├── stubs.go                # MODIFY — remove newStatusCmd() (now in status.go)
├── sync.go
├── template.go
└── upgrade.go

internal/fleet/
├── add.go
├── add_test.go
├── deploy.go
├── deploy_test.go
├── diagnostics.go          # MAYBE MODIFY — add hint patterns for repo_inaccessible / rate_limited if not already covered
├── diagnostics_test.go
├── errors.go
├── errors_test.go
├── execlog.go
├── execlog_test.go
├── fetch.go                # READ-ONLY — same-package access; ghAPIRaw / ghAPIJSON are reachable without export (see Structure Decision below)
├── frontmatter.go          # READ-ONLY — SplitFrontmatter / ParseFrontmatter reused as-is
├── list_result.go
├── list_result_test.go
├── load.go
├── schema.go
├── status.go               # NEW — Status(ctx, cfg, opts) entrypoint, RepoStatus, WorkflowDrift, worker pool
├── status_test.go          # NEW — table-driven diff fixtures (no network)
├── sync.go
└── upgrade.go

# Documentation
README.md                   # MODIFY — add `status` to the CLI surface examples
CHANGELOG.md                # NOT TOUCHED MANUALLY — release-please derives from conventional commits
```

**Structure Decision**: Same-package design — `internal/fleet/status.go` lives alongside `fetch.go`, `frontmatter.go`, `diagnostics.go` so it can call `ghAPIRaw` / `ghAPIJSON` / `SplitFrontmatter` / `CollectHints` without an export-API surface change. `cmd/status.go` is the thin Cobra wrapper, mirroring the `cmd/deploy.go` ↔ `internal/fleet/deploy.go` split. **No new packages, no helper-extraction phase.** This is the minimum-surface design that preserves the project's existing layering; any other split would be premature abstraction.

## Complexity Tracking

> **No constitution violations — this section intentionally empty.**

The design uses zero new third-party dependencies, introduces zero new packages, reuses every cross-cutting helper (`gh api` wrappers, frontmatter parsing, hints collection, JSON envelope), and adds two new files plus minor edits to two existing ones. There is no abstraction to justify — the design is the simplest thing that meets every functional requirement, every success criterion, and every clarification answer.
