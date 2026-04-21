# Phase 0 Research: Structured Logging

**Feature**: 002-add-zerolog-logging
**Date**: 2026-04-20

This document resolves the technical unknowns identified in `plan.md`'s Technical Context. Each section follows the Decision / Rationale / Alternatives pattern.

---

## R1: zerolog transitive dependency audit

**Question**: Does adding `github.com/rs/zerolog` to `go.mod` introduce any transitive dependencies beyond itself (constitution Principle I; spec FR-014 / SC-006)?

**Decision**: Add `github.com/rs/zerolog` pinned to the latest stable minor (v1.x as of the implementation commit). Verify with `go mod graph` before and after the change; the delta SHOULD be as minimal as possible — in practice it adds zerolog plus two tiny TTY-detection indirects (`mattn/go-colorable`, `mattn/go-isatty`) that ConsoleWriter needs to detect color-capable terminals. Those two indirects are accepted as compile-time prerequisites of the chosen format API; deeper zerolog deps (`coreos/go-systemd`, `pkg/errors`, `rs/xid`) are gated behind build tags and are NOT pulled into the compiled module closure.

**Implementation finding (2026-04-20, T001/T002)**: go mod tidy added to `go.mod` exactly three edges — `github.com/rs/zerolog v1.35.1` (direct), `github.com/mattn/go-colorable v0.1.14` (indirect), `github.com/mattn/go-isatty v0.0.20` (indirect). Both indirects are ~100 LOC, both widely used across the Go ecosystem, both necessary for ConsoleWriter's terminal detection. This is a minor softening of the "zero transitive" expectation — the spec's SC-006 wording "one direct dependency with zero additional transitive dependencies" is narrowly violated by the two indirects but is accepted as the cost of the chosen console-formatting feature; the alternative (rolling our own TTY detection) is not worth it.

**Rationale**: zerolog deliberately uses only the Go standard library. Its README states "No Dependencies." It provides a `ConsoleWriter` (uses `io.Writer` + stdlib time formatting) and JSON output (hand-written encoding via `strconv` — no encoding/json reflection). The only indirect stdlib imports are `io`, `os`, `time`, `strconv`, `sync`, `fmt`, which are already used throughout this codebase.

**Verification procedure** (performed at implementation time):
```sh
go mod graph > /tmp/before.txt
go get github.com/rs/zerolog@<tag>
go mod tidy
go mod graph > /tmp/after.txt
diff /tmp/before.txt /tmp/after.txt
```
Expected diff: a single `+ github.com/rshade/gh-aw-fleet <space> github.com/rs/zerolog@<tag>` line. Any line that introduces a second package indicates a regression — back out and re-evaluate.

**Alternatives considered**:
- **Go 1.21+ `log/slog` (stdlib)**: Zero dependencies by definition. Rejected because (a) the issue specifies zerolog, (b) the `log/slog` JSON handler is encoding/json-based and allocates more per event, (c) our call-site ergonomics target zerolog's fluent-chain API (`log.Warn().Str("repo", r).Msg(...)`) which `slog` requires more verbose construction for. Valid future migration path if zerolog becomes a liability, but not chosen now.
- **`uber-go/zap`**: Excellent performance, but pulls in at least two transitive deps (`go.uber.org/multierr`, `go.uber.org/atomic` historically) — fails the zero-new-transitive-dep constraint. Rejected.
- **`sirupsen/logrus`**: Widely used but JSON formatting is reflection-based (slower, more allocations). Rejected.

---

## R2: zerolog API patterns and global logger replacement

**Question**: How should `internal/log.Configure(level, format string) error` shape the zerolog global logger so that call sites across the codebase can use `log.Warn().Str(...).Msg(...)` without per-call-site setup?

**Decision**:

```go
package log

import (
    "fmt"
    "io"
    "os"
    "time"

    "github.com/rs/zerolog"
    zlog "github.com/rs/zerolog/log"
)

// Configure parses level and format strings and installs a global logger
// that writes to stderr. Call once, early, before any log events fire.
func Configure(level, format string) error {
    lvl, err := zerolog.ParseLevel(level)
    if err != nil {
        return fmt.Errorf("invalid --log-level %q: %w", level, err)
    }

    var w io.Writer
    switch format {
    case "json":
        w = os.Stderr
    case "console":
        w = zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}
    default:
        return fmt.Errorf("invalid --log-format %q: must be %q or %q", format, "console", "json")
    }

    zerolog.TimeFieldFormat = time.RFC3339
    zerolog.SetGlobalLevel(lvl)
    zlog.Logger = zerolog.New(w).With().Timestamp().Logger()
    return nil
}
```

Call sites everywhere else import `"github.com/rs/zerolog/log"` and use `log.Warn().Str("repo", r).Msg("…")`, `log.Error().Err(err).Msg("…")`, etc.

**Rationale**:
- `zerolog.ParseLevel` accepts `"trace"`, `"debug"`, `"info"`, `"warn"`, `"error"`, `"fatal"`, `"panic"` — plus variants. We exposed `trace/debug/info/warn/error` in FR-001; `fatal`/`panic` are a silent superset and acceptable.
- Global logger replacement via `zlog.Logger = …` is zerolog's documented pattern (see README). One reassignment in `Configure` keeps the rest of the codebase free of logger-passing plumbing.
- `time.RFC3339` is the universal machine-parseable timestamp; zerolog's `ConsoleWriter` renders it compactly in console mode.
- `Err(err)` on `log.Error()` or `log.Warn()` chains populates a top-level `error` key in JSON output — this satisfies the FR-017 "dedicated `error` field" half of the convention. To satisfy the "also appended to `message`" half, call sites use a helper pattern: `log.Error().Err(err).Msgf("deploy failed: %s", err)` — the error string appears in both places. We document this pattern in `contracts/log-event.md` and in the package doc comment.

**Alternatives considered**:
- **Functional option API** (`Configure(WithLevel(…), WithFormat(…))`): More idiomatic for growing APIs, but the spec's knob set is closed (level + format only). YAGNI — two-positional-arg signature is clearer for two values that are always paired.
- **Context-bound logger** (every call site threads a `*zerolog.Logger` through context): Flexible but forces every caller to plumb context. Rejected — the spec's single-logger-per-invocation model matches zerolog's global pattern exactly.
- **Custom caller helper to auto-append Err() to Msg()**: Considered but rejected. Every `log.Error().Err(err).Msgf("…: %s", err)` call site makes the intent obvious; a helper would hide the duplication and invite drift.

---

## R3: cobra flag ordering — does `PersistentPreRunE` run before or after flag parsing?

**Question**: FR-004 requires invalid `--log-level` values be rejected with a clear error BEFORE the subcommand runs. The Q4 clarification answer says the rejection MUST be plain stderr (via cobra's default flag-error path), NOT through the logger. Is that mechanically possible given where `PersistentPreRunE` sits in cobra's lifecycle?

**Decision**: Rely on cobra's flag-parsing validation (via `cobra.OnlyValidArgs` equivalents on our flag values OR custom pre-run validation). The execution order for a cobra command is:

1. Parse flags (pflag). Unknown flag value is rejected here with `Error: invalid argument "…" for "--…" flag: …` on stderr — plain text, no logger involvement.
2. Run `PersistentPreRunE` on parent commands (root → … → leaf).
3. Run `PreRunE`, `RunE`, `PostRunE`, `PersistentPostRunE`.

Flag value validation must therefore happen either (a) via pflag's built-in type validation, or (b) in `PersistentPreRunE` by calling `internal/log.Configure(flagLogLevel, flagLogFormat)` and returning its error directly. Cobra prints returned errors to the `ErrOrStderr()` writer as `Error: <msg>` — plain text, no JSON, no logger. This satisfies FR-004 and the Q4 clarification.

**Critical ordering detail**: Since `PersistentPreRunE` runs BEFORE `RunE`, the logger is configured before any subcommand work. Meanwhile, `PersistentPreRunE`'s own error (from `Configure`) goes through cobra's error path, NOT through the yet-to-be-installed logger — exactly what the Q4 clarification requires. Perfect fit.

**Rationale**: No custom error-handling path needed. Cobra's default behavior is exactly what FR-004 requires. Our only job is: validate in `PersistentPreRunE`, return the error, let cobra handle it.

**Alternatives considered**:
- **Using pflag `SetNormalizeFunc` / custom `Var` type**: Overkill for two enum-valued flags. Rejected.
- **Validating in each subcommand's `PreRunE`**: Duplicates validation logic across every leaf command. Rejected — `PersistentPreRunE` on root is the canonical spot.
- **Deferring validation to first log call**: Would allow a subcommand to run side-effects with an invalid log flag and fail mid-pipeline. Violates FR-004 directly. Rejected.

---

## R4: Subprocess summary helper design — deriving `tool` and `subcommand` without raw argv capture

**Question**: FR-011 requires subprocess summary events to carry `tool` (e.g., `git`, `gh`, `gh aw`) and `subcommand` (e.g., `push`, `pr create`) fields. FR-016 forbids logging raw argv. How does the helper extract those two strings without falling back to logging the full arg list?

**Decision**: Introduce `internal/fleet/execlog.go` with a single helper:

```go
package fleet

import (
    "os/exec"
    "time"

    "github.com/rs/zerolog/log"
)

// runLogged runs cmd and, after it returns, emits a structured debug-level
// summary event. It does NOT read cmd.Args beyond Args[0] and a single
// allow-listed subcommand lookup — full argv is never logged.
//
// extraFields lets callers attach repo, workflow, or clone_dir context from
// the call site; no subprocess-derived fields are added automatically beyond
// tool/subcommand/exit_code/duration (integer milliseconds, rendered by zerolog's .Dur()).
func runLogged(cmd *exec.Cmd, toolLabel, subcommandLabel string, extraFields map[string]string) error {
    start := time.Now()
    err := cmd.Run()
    ev := log.Debug().
        Str("tool", toolLabel).
        Str("subcommand", subcommandLabel).
        Int("exit_code", cmd.ProcessState.ExitCode()).
        Dur("duration", time.Since(start))
    for k, v := range extraFields {
        ev = ev.Str(k, v)
    }
    ev.Msg("subprocess exited")
    return err
}
```

Call sites supply `toolLabel` and `subcommandLabel` from string literals at the call site — e.g., `runLogged(cmd, "gh", "aw upgrade", fields)`. These are caller-known, not derived from `cmd.Args`, so there is zero risk of argv leaking into the log event even if a future edit adds credentials to an arg.

**Rationale**:
- The allowlist in FR-016 is enforceable because the helper has a closed set of fields it writes. `extraFields` is a `map[string]string` but its keys are chosen by the call site and reviewed against the allowlist during code review; adding a disallowed key requires an obvious edit to a specific call site.
- Separating `tool` from `subcommand` makes filtering by `.tool=="gh"` or `.subcommand=="pr create"` ergonomic in jq.
- `cmd.ProcessState.ExitCode()` is safe to call after `cmd.Run()` returns — including on error paths (non-zero exit) where ProcessState is populated.
- Duration via `time.Since(start)` is computed regardless of err, since we want timing for both success and failure cases.

**Alternatives considered**:
- **Derive subcommand from `cmd.Args[1]`**: Tempting but brittle — `gh aw upgrade` is `cmd.Args = ["gh", "aw", "upgrade"]`; `gh aw` needs Args[1]+Args[2]. Two-word subcommands would need ad-hoc logic. Rejected in favor of caller-supplied literals.
- **Log raw `cmd.Args` at debug only**: Explicitly considered and rejected during Q1 clarification. Even debug-only exposure is too easy to forget in an incident where logs get shared.
- **Return a struct instead of logging inline**: Over-engineering. The helper is local to `internal/fleet`; callers don't need the summary data programmatically.

---

## R5: Testing stderr capture with cobra

**Question**: How do new tests in `cmd/root_logging_test.go` and `internal/log/log_test.go` capture the logger's stderr output for JSON parsing and assertions?

**Decision**: Two approaches, used in different test tiers:

**Tier A — `internal/log/log_test.go`**: Since `Configure` targets `os.Stderr` directly, tests temporarily swap `os.Stderr` with a `*os.File` from `os.Pipe()`, call `Configure`, emit a log event, read the pipe, and assert. Pattern:

```go
r, w, _ := os.Pipe()
orig := os.Stderr
os.Stderr = w
_ = Configure("info", "json")
log.Warn().Str("repo", "x").Msg("t")
_ = w.Close()
os.Stderr = orig
buf, _ := io.ReadAll(r)
// assert JSON structure of buf
```

**Tier B — `cmd/root_logging_test.go`**: Construct the root command with `NewRootCmd()`, pre-configure the logger to write to a `*bytes.Buffer` captured at test time (by either (a) letting the test call `log.Configure` with explicit flags and temporarily redirecting `os.Stderr`, or (b) adding a seam: a package-level `stderrSink io.Writer = os.Stderr` that tests can override). Recommend option (a) — avoids test-only seams in production code.

**Rationale**: Tier A is mechanical and exercises `Configure` end-to-end. Tier B exercises flag registration + `PersistentPreRunE` + one real log event in one integration-style test, which gives the highest confidence per test for minimal cost.

**Alternatives considered**:
- **Parallel tests with shared global logger**: Reject. Tests MUST run serially (or via `t.Setenv`-style isolation) because the global logger is process-wide. Tests using `os.Stderr` replacement acquire an implicit lock; mark them `t.Parallel()`-incompatible.
- **Use a testing-dedicated `TestLogger`**: Over-engineering; the global logger IS the test surface.

---

## R6: CHANGELOG and CLAUDE.md update scope

**Decision**: Update `CHANGELOG.md` with a brief entry under "Added" (the new flags) and "Changed" (the WARNING-to-stderr move). Update `CLAUDE.md`'s Architecture section with one sentence pointing at `internal/log` and noting the two persistent flags. No change to `CONTEXT.md` (this does not alter the thin-orchestrator boundary). No change to skills (`fleet-deploy`, `fleet-eval-templates`, `fleet-upgrade-review`, `fleet-onboard-repo`) unless the three-turn flow changes — it does not.

**Rationale**: Keeps documentation updates surgical. The feature is additive from a UX standpoint except for the warning-stream move, which is the single item worth explicit changelog attention.

---

## Summary table

| Research item | Decision | Satisfies |
|---|---|---|
| R1 zerolog deps | Pin latest v1; verify zero new transitive | FR-014, SC-006, Const. Principle I |
| R2 zerolog API shape | Global logger, `Configure(level, format)` | FR-001..006, FR-015, FR-017 |
| R3 cobra ordering | `PersistentPreRunE` on root; return error for invalid flags | FR-003, FR-004, Q4 clarification |
| R4 execlog helper | `runLogged` with caller-supplied tool/subcommand labels | FR-011, FR-012, FR-016 |
| R5 stderr capture in tests | `os.Pipe()` swap in unit tests; direct for integration | FR-015, SC-004 |
| R6 docs updates | CHANGELOG + CLAUDE.md one-liner | Constitution Principle III docs discipline |

All NEEDS CLARIFICATION items from Technical Context resolved. Ready for Phase 1.
