# Phase 0 Research: Compile Workflows with --strict on Public Repos

**Date**: 2026-05-17
**Spec**: [spec.md](./spec.md)
**Plan**: [plan.md](./plan.md)

All clarifications from the spec are resolved (Session 2026-05-17). This document records the **research decisions** that close the loop on each Technical Context choice — specifically the API surfaces and parse strategies the plan depends on.

## R1 — `gh api /repos/<owner>/<repo>` visibility field

**Decision**: Use the response's `visibility` field (string), select the literal `"public"` for auto-on, treat every other value (including `"private"`, `"internal"`, and any future enum value) as auto-off.

**Rationale**: Confirmed empirically on 2026-05-17 against `rshade/gh-aw-fleet` — the response contains `"visibility":"public"` as a top-level string. The field has been stable in the GitHub REST API since v3 and is the canonical successor to the legacy `"private":<bool>` field (which is still present but cannot distinguish `private` from `internal`). Selecting `"public"` only (rather than negating `"private"`) preserves the spec's Edge Case that `internal` repos are not subject to the public-repo runtime check.

**Alternatives considered**:

- **Use `"private":false`**: rejected. Loses the `internal` distinction; an enterprise-org `internal` repo would falsely auto-enable strict mode.
- **Use `gh repo view --json visibility`**: rejected. Adds a second gh subprocess shape for the same data; `gh api` is the existing precedent (see `ghAPIExists` / `ghAPIJSON` call sites).

**Implementation note**: Reuse the existing `ghAPIJSON` package-level seam to fetch the JSON and extract `.visibility` via type assertion against `map[string]any`. The seam supports test injection without new infrastructure.

## R2 — `gh aw compile --help` probe parser

**Decision**: Combined stdout+stderr capture of `gh aw compile --help`, scanned with `strings.Contains(out, "--strict")`. The probe returns three outcomes:

1. **`flag-present`**: probe exit 0 AND output contains `--strict` → proceed to compile.
2. **`flag-absent`**: probe exit 0 AND output lacks `--strict` → abort with "gh aw too old" diagnostic.
3. **`probe-failed`**: probe exit non-zero OR exec error (binary missing) → abort with "gh aw not found / broken" diagnostic.

**Rationale**: Confirmed empirically that v0.72.1's help output includes the `--strict` flag and its description in a single line — substring match is robust to formatting variations (column width changes, prefix flag-letter additions, future help-renderer changes that preserve the canonical flag name). `flag.NewFlagSet` parsing was considered but adds parser complexity for no benefit: we only care about presence/absence of one token.

**Alternatives considered**:

- **Parse with `flag.NewFlagSet`**: rejected. The help output is gh-aw's own formatter, not stdlib flag's; would require maintaining a parser for an external format we don't own. Substring match is correct and stable.
- **Probe `gh aw --version` instead and compare to `v0.68.3`**: rejected. Adds a semver comparison library (or hand-rolled comparison logic) and assumes the minimum version we hardcode matches the version when `--strict` was added. Probing for the flag directly answers the actual question without coupling.
- **Skip the probe; let compile fail with stderr**: rejected by Q2 → Option A clarification. The point of the probe is to convert a noisy "unknown flag: --strict" stderr into a clean precondition diagnostic.

**Implementation note**: The probe is invoked via a package-level `var ghAwCompileHelp = func(ctx context.Context) (string, error) { ... }` seam. Tests inject fixtures returning `"... --strict ..."` (flag-present) / `"... --some-other-flag ..."` (flag-absent) / `error` (probe-failed).

## R3 — `gh aw --version` parser for diagnostic enrichment

**Decision**: Capture stdout of `gh aw --version`, extract the first token matching `v\d+\.\d+\.\d+` with `regexp.MustCompile(\`v\d+\.\d+\.\d+\`)`, surface it in the "gh aw too old" diagnostic as best-effort context. When parsing fails, the diagnostic still fires with `(version unknown)` substituted.

**Rationale**: Confirmed empirically on 2026-05-17 — output format is `gh aw version v0.72.1`. The regex anchors on the `v` prefix to avoid false positives on numerals elsewhere in the output. The diagnostic does NOT *gate* on the parsed version (the probe already failed by the time we reach this code); the version string is contextual information for the operator's remediation message.

**Alternatives considered**:

- **Require semver parsing via `golang.org/x/mod/semver`**: rejected. Adds a dependency to the indirect-but-not-direct list (it's stdlib-adjacent but still external). Best-effort regex is sufficient for a diagnostic message.
- **Omit the detected version entirely**: rejected. Operators benefit from seeing "your gh aw is v0.50.0; minimum is v0.68.3" in one glance, rather than having to run `gh aw --version` themselves.

**Implementation note**: Like R2, behind a package-level `var ghAwVersion = func(ctx context.Context) (string, error) { ... }` seam for test injection.

## R4 — Visibility-lookup error classification

**Decision**: Any non-`nil` error from `ghRepoVisibility` and any response where the `visibility` field is missing, non-string, or absent from the JSON map → classified as `auto-fallback`. The resolver returns `(true, "auto-fallback", truncate(err.Error(), 200))` so the caller can fail-secure (strict ON) AND emit the FR-007 `warn` log line carrying the truncated reason. The 200-char truncation prevents leaking large network errors into log streams. The resolver itself never returns an error — fail-secure semantics fold the lookup error into the `auto-fallback` source + `reason` string (see FR-003 and data-model.md §E2).

**Rationale**: The spec's Acceptance Scenario 1 for US3 explicitly enumerates the failure modes (403, 404, 5xx, network error, malformed JSON, missing field) and unifies them under one fail-secure outcome. There's no operator value in distinguishing "field missing" from "HTTP 5xx" — both produce the same actionable response: set `compile_strict` explicitly to bypass the auto-detection.

**Alternatives considered**:

- **Distinguish HTTP 403 from network error**: rejected. Spec 005 (Actions preflight) also collapses these into a single `indeterminate` outcome for the same operator-facing reason. Consistency with the existing pattern wins.
- **Retry with backoff**: rejected. The spec is silent on retry, and adding implicit retry would mask transient issues the operator should see. If the visibility lookup is unreliable enough to need retries, the operator should set explicit `compile_strict` per-repo.

**Implementation note**: The error message in the `warn` log uses the truncated raw error; the structured event includes a `reason` field with the same content for log-stream consumers.

## R5 — `diagnostics.CollectHints` hint patterns

**Decision**: Add three new hint pattern entries in `internal/fleet/diagnostics.go` (registered against the existing `CollectHints` registry):

1. **`DiagCompileStrictFailed`**: triggered when compile stderr contains either `"strict mode validation"` or `"strict mode requires"` (covers gh-aw's current and likely-future error phrasings). The hint names the `compile_strict: false` opt-out path and points operators at the work-dir clone for inspection.
2. **`DiagGhAwTooOld`**: triggered when the FR-016 probe returns `flag-absent`. The hint names the minimum version (`v0.68.3`), the detected version (R3 output, or `(version unknown)`), and a one-line upgrade command (`gh extension upgrade aw`).
3. **`DiagGhAwMissing`**: triggered when the probe itself fails (exec error or non-zero exit). The hint distinguishes "binary not found" (suggest `gh extension install github/gh-aw`) from "exec error" (raw stderr included).

**Rationale**: Three distinct diagnostic codes preserve the operator-facing precision of the existing diagnostics (`DiagHint`, `DiagHTTP404`, `DiagMissingSecret`). Combining "too old" and "missing" into one code would force the message to be vaguer; the codes are cheap and improve the JSON envelope's `warnings[].code` consumer surface.

**Alternatives considered**:

- **One `DiagCompileStrictPrecondition` code**: rejected. The remediation is different for each failure: upgrade (too old), install (missing), or `compile_strict: false` (validation failed). One code with multiple `fields` was considered but loses the existing pattern's clarity.
- **Match on regex against compile stderr**: rejected as overkill — substring match is robust to formatting changes in gh-aw stderr.

**Implementation note**: The new codes follow the existing constant-block layout in `diagnostics.go` (e.g., `DiagCompileStrictFailed = fleetdiag.DiagCompileStrictFailed` re-export). The matching is folded into `CollectHints(texts...)` as additional `if strings.Contains(...)` branches; no new entry-point function.

## R6 — Log message contract for FR-006 / FR-007

**Decision**: Use zerolog structured fields matching the codebase's existing event-naming convention:

```go
// FR-006 info line (resolver succeeded with any outcome):
log.Info().
    Str("event", "compile_strict_resolved").
    Str("repo", repo).
    Bool("effective", effective).
    Str("source", source). // "explicit" | "auto-public" | "auto-private" | "auto-fallback"
    Msg("compile-strict resolution")

// FR-007 warn line (fail-secure fallback):
log.Warn().
    Str("event", "compile_strict_lookup_failed").
    Str("repo", repo).
    Str("reason", truncatedErr). // ≤200 chars
    Msg("compile-strict visibility lookup failed; defaulting to strict ON")
```

**Rationale**: Matches the existing zerolog usage pattern in `internal/log` and `internal/fleet/load.go` (see `hujson_fallback_to_rewrite` warn event). The `event` field is the stable key CI consumers use to filter; the `Msg` is a human-readable summary. Snake-case event names match the existing codebase convention.

**Alternatives considered**:

- **Free-form log messages without structured fields**: rejected. Inconsistent with the rest of the codebase; harder to grep for in operator log-tailing.
- **Different event names per resolution-source branch**: rejected. One canonical event name (`compile_strict_resolved`) keeps log consumers' filter rules simple; the discriminant lives in the `source` field.

**Implementation note**: Caller-side zerolog field setup is already wired via `internal/log.Configure`; no infrastructure work needed.

## Resolved unknowns summary

| Spec NEEDS CLARIFICATION marker | Resolution |
|---------------------------------|------------|
| (Q1 in original spec) — JSON envelope contract | Resolved in clarification session: Option A (typed fields `CompileStrictApplied` + `CompileStrictSource` on `DeployResult` / `UpgradeResult`). |
| (Q2 in original spec, raised during clarify) — local `gh aw` version handling | Resolved: Option A (probe `gh aw compile --help` once when strict is effective; abort cleanly on flag-absent or probe-failed). |

No unresolved unknowns remain. Phase 1 (Design & Contracts) can proceed.
