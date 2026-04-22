# Implementation Plan: Structured Logging for Errors, Warnings, and Diagnostics

**Branch**: `002-add-zerolog-logging` | **Date**: 2026-04-20 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `specs/002-add-zerolog-logging/spec.md`

## Summary

Add a thin structured-logging layer (package `internal/log`, backed by `github.com/rs/zerolog`) that emits errors, warnings, subprocess summaries, and diagnostic hints to stderr in either human-readable (`console`) or JSON form, controlled by two new persistent CLI flags (`--log-level`, `--log-format`). Human-readable tabwriter status output on stdout stays untouched on warning-free paths. Existing `⚠ WARNING:` lines in `cmd/deploy.go` and `cmd/sync.go` move to `log.Warn()` with structured fields (`repo`, etc.). Subprocess invocations are wrapped by a small helper that logs one `debug`-level summary event after each `exec.Cmd.Run()`. Diagnostic hints from `internal/fleet/diagnostics.go` become structured `warn`-level events with the hint text in a dedicated field. Secrets are kept out of log fields by a fixed allowlist — raw argv, env, and URL-bearing strings are explicitly excluded.

## Technical Context

**Language/Version**: Go 1.25.8 (from `go.mod`).
**Primary Dependencies**: `github.com/spf13/cobra` v1.10.2 (existing), `github.com/rs/zerolog` (new — pin a recent tagged release at implementation time; verify zero transitive deps beyond stdlib).
**Storage**: N/A (logging is stderr-only; no log files, no rotation, no remote shipping — spec Out of Scope).
**Testing**: Go standard `testing` package. New tests live in `internal/log/log_test.go` and `cmd/root_logging_test.go` (a new file, since `cmd/` has no existing `_test.go` today). Existing integration tests in `internal/fleet/*_test.go` must continue to pass.
**Target Platform**: Cross-platform CLI (Linux, macOS, Windows) — same platforms the `gh` extension already runs on.
**Project Type**: Single-project CLI.
**Performance Goals**: Logging overhead MUST be imperceptible in human time (sub-millisecond per event). zerolog's zero-allocation call path satisfies this; level filtering MUST short-circuit structured-field evaluation at the default `info` level so debug-level `Debug().Str(...).Msg(...)` chains cost nothing in production invocations.
**Constraints**:
- Zero new transitive dependencies (thin-orchestrator principle; verified in Phase 0 research).
- Stdout output on warning-free command invocations MUST remain byte-identical to the pre-feature baseline (spec FR-008, SC-001).
- Structured log fields MUST stay inside the FR-016 allowlist; no raw argv/env/URL-bearing strings.
- gpg signing, git-from-Bash-tool deny, three-turn mutation pattern are untouched (no implicated surface).
**Scale/Scope**: ~5 files in `cmd/` touched (root, deploy, sync, upgrade, main.go via root), ~5 files in `internal/fleet/` touched (deploy, upgrade, sync, fetch, diagnostics — sync consumes via deploy). ~12 subprocess `exec.CommandContext` sites wrapped. One new package (`internal/log`) with 2 files (impl + test). One new helper file in `internal/fleet` (`execlog.go`) for subprocess summary emission.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Evaluated against `.specify/memory/constitution.md` version 1.0.0.

### I. Thin-Orchestrator Code Quality — **PASS**

- zerolog is a single direct dependency. The constitution permits dependencies whose purpose is orthogonal to upstream tools (`gh aw`, `gh`, `git`). Logging is orthogonal — it does not duplicate, wrap, or replace any behavior of the three upstream tools. **Verification requirement**: Phase 0 research confirms `go mod graph` adds exactly one new node (no transitive deps).
- `go build ./...` and `go vet ./...` must stay clean. New code follows Go idioms (`%w` wrapping, no unused imports, self-documenting names). No file exceeds the ~300-line guidance: `internal/log/log.go` will be ~80 LOC; `internal/fleet/execlog.go` will be ~50 LOC; changes elsewhere are localized edits, not expansions.
- Comments only explain WHY — specifically why flag validation bypasses the logger (FR-004), why the subprocess summary emits at `debug` (FR-011), why fields are allowlisted (FR-016). WHAT-level comments are avoided.

### II. Testing Standards (Build-Green + Real-World Dry-Run) — **PASS**

- `go build ./...` and `go vet ./...` MUST pass. Trivially satisfied — nothing in the change should break either.
- Existing dry-run surface (`deploy` / `sync` / `upgrade` with no `--apply`) continues to exercise real `gh aw` against a scratch clone. This feature adds structured output; the dry-run mechanism is unchanged. `fleet.CollectHints` still runs inside the dry-run; we additionally log hints as structured events alongside the existing `hint:` tabwriter lines.
- Skills in `skills/*/SKILL.md` — none need re-testing since the command surface is additive (new flags, same behavior at defaults). A brief note should be added to `skills/fleet-deploy/SKILL.md` that `--log-level=debug` is now a debugging aid for failures; this is a doc tweak, not a skill-logic change.
- New unit tests: `internal/log/log_test.go` covers the full `Configure(level, format)` contract (FR-001..006, FR-015) including invalid inputs. `cmd/root_logging_test.go` covers persistent-flag registration and end-to-end capture of a warning through the JSON path (FR-009, FR-017 for error events).

### III. User Experience Consistency (Three-Turn Mutation Pattern) — **PASS**

- The three-turn pattern (dry-run → approval → apply) is not touched. Flags are strictly additive; defaults preserve current behavior.
- Conventional Commits with `ci(workflows)` scope in `internal/fleet/*.go` — unchanged.
- Every recoverable failure still routes through `fleet.CollectHints`. This feature layers a *structured* emission path on top of the existing plaintext `hint:` line — it does not remove the hint line from the dry-run output (which would change stdout on failure paths, violating SC-001 for dry-runs that hit hint-eligible errors).
- Scratch clones at `/tmp/gh-aw-fleet-*` — unchanged.
- **UX shift (documented)**: On warning paths (deploy with missing secret, sync with drift), stdout no longer contains the `⚠ WARNING:` block — it moves to stderr as a structured event. This is intentional per spec FR-009. CHANGELOG.md MUST flag this as a user-visible change (low-risk: operators still see the warning, just on stderr; scripts that parsed stdout for `⚠ WARNING:` need to switch to stderr).

### IV. Performance Requirements — **PASS**

- Parallelism / caching surface is untouched.
- zerolog's level-filtering happens before field evaluation, so debug calls cost ~nothing at `info` level. Subprocess summary adds one `time.Since()` + one log call per `exec` — negligible relative to the subprocess itself (which takes ms–s).
- No command regresses past the 5-minute ceiling.

### Declarative Reconcile Invariants — **PASS**

- No new persistent state. Logging is stderr-only.
- `fleet.json` / `fleet.local.json` layering untouched.
- gpg-signing and `git add|commit|push`-from-Bash denials untouched.
- `github/gh-aw` pinning rules untouched.

### Gate verdict

All gates pass on initial check. **Complexity Tracking table is intentionally empty — no violations to justify.**

## Project Structure

### Documentation (this feature)

```text
specs/002-add-zerolog-logging/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   ├── cli-flags.md     # --log-level / --log-format contract
│   └── log-event.md     # Field allowlist + JSON shape contract
├── checklists/
│   └── requirements.md  # Spec quality checklist (from /speckit.specify)
├── spec.md              # Feature specification
└── tasks.md             # (Phase 2 — created by /speckit.tasks, NOT by this command)
```

### Source Code (repository root)

```text
cmd/
├── root.go              # +PersistentPreRunE, +flagLogLevel, +flagLogFormat
├── deploy.go            # MissingSecret ⚠ WARNING → log.Warn().Str("repo", ...).Msg(...)
├── sync.go              # Drift ⚠ WARNING → log.Warn().Str("repo", ...).Strs("drift", ...).Msg(...)
├── upgrade.go           # (no existing ⚠ WARNING today; hints still surface via tabwriter + now also log)
├── list.go              # unchanged
├── add.go               # unchanged
├── template.go          # unchanged
├── stubs.go             # unchanged
└── root_logging_test.go # NEW — persistent-flag registration + warning-through-JSON capture

main.go                   # fmt.Fprintln(os.Stderr, err) → log.Error().Err(err).Msg("fatal")

internal/
├── log/                  # NEW PACKAGE
│   ├── log.go            # Configure(level, format string) error
│   └── log_test.go       # levels, formats, invalid inputs, default quiet
└── fleet/
    ├── execlog.go        # NEW — runLogged(ctx, cmd, summaryFields) wraps exec.Cmd.Run() + emits debug summary
    ├── deploy.go         # each exec.CommandContext(...).Run() routed through runLogged (except where already wrapped)
    ├── upgrade.go        # same; existing io.MultiWriter(os.Stderr, &buf) patterns preserved for live tee
    ├── sync.go           # (delegates to deploy helpers; minimal edits)
    ├── fetch.go          # 2× gh api sites wrapped
    ├── diagnostics.go    # CollectHints output is logged structurally at call sites (cmd/deploy.go, cmd/sync.go, cmd/upgrade.go) with hint text in a field

go.mod                    # +github.com/rs/zerolog
go.sum                    # updated via go mod tidy

CHANGELOG.md              # "Added: --log-level, --log-format flags. Changed: ⚠ WARNING lines moved from stdout to stderr (structured log.Warn events)."
CLAUDE.md                 # +one-line entry under Architecture describing the logging layer
```

**Structure Decision**: Single-project CLI layout. New package `internal/log` provides the `Configure(level, format)` entry point; call sites everywhere else use `github.com/rs/zerolog/log` global API directly (no re-export). New helper `internal/fleet/execlog.go` centralizes subprocess-wrap logic so all 12-ish exec sites get consistent summary emission without per-site boilerplate.

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

No violations. Table intentionally empty.

## Phase 0: Outline & Research

See [research.md](research.md).

Research items resolved:
1. zerolog transitive-dependency audit (constitution Principle I, spec FR-014/SC-006).
2. zerolog API patterns — `Configure`, `ConsoleWriter`, `Err()` field rendering, global logger replacement.
3. cobra `PersistentPreRunE` ordering vs flag parsing — confirms invalid-flag rejection reaches stderr via cobra's standard error path before `PersistentPreRunE` would have configured the logger (satisfies FR-004 + Q4 clarification).
4. Subprocess wrapper design — how to derive `tool` and `subcommand` fields from `exec.Cmd.Args` without capturing raw argv (satisfies FR-011 + FR-016).
5. Test strategy for stderr capture with `cobra.Command.SetErr(...)` and stderr-teed logger output.

No NEEDS CLARIFICATION markers remain in Technical Context after Phase 0.

## Phase 1: Design & Contracts

**Prerequisites**: `research.md` complete.

Outputs:
- [data-model.md](data-model.md) — the three feature entities (Log event, Logger configuration, Subprocess summary, Diagnostic hint) with field names, types, allowlist membership, and source-of-truth per field.
- [contracts/cli-flags.md](contracts/cli-flags.md) — the persistent-flag contract: name, type, default, valid values, exit behavior on invalid.
- [contracts/log-event.md](contracts/log-event.md) — the JSON shape contract: required fields, optional fields, forbidden fields, `error` field convention per FR-017.
- [quickstart.md](quickstart.md) — a minimal local reproduction: build, run all flag combinations against `list`, verify stdout byte-identity on default, verify stderr JSON parseability with `jq`, verify a warn event structure.

Agent context update: run `.specify/scripts/bash/update-agent-context.sh claude` so `CLAUDE.md` picks up the new `internal/log` package reference and the two new persistent flags.

### Constitution re-check (post-design)

All four principles + Declarative Reconcile Invariants re-evaluated against the data model and contracts — no new violations introduced. Verdict: still **PASS**, no complexity justifications required.
